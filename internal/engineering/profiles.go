package engineering

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// ProfileRepository persists evidence/checkpoints, not an alternative execution
// queue. Domain publication and the shared operation fence commit atomically.
type ProfileRepository interface {
	ReserveProfile(context.Context, identity.Envelope, ProfileRecord) (ProfileRecord, error)
	ReadProfile(context.Context, identity.Envelope, string, bool) (ProfileRecord, error)
	AttachProfile(context.Context, identity.Envelope, string, jobs.RequestTask) (ProfileRecord, error)
	StartProfileRead(context.Context, jobs.Invocation, ProfileRecord, string, time.Time) error
	CheckpointProfile(context.Context, jobs.Invocation, ProfileRecord, Profile) error
	StartProfileSummary(context.Context, jobs.Invocation, ProfileRecord) (bool, error)
	PublishProfile(context.Context, jobs.Invocation, ProfileRecord, Profile) error
	ProfileHistory(context.Context, identity.Envelope, string, string, string, int) ([]ProfileStatus, error)
	ProfileEvidence(context.Context, identity.Envelope, string) (ProfileEvidence, error)
	RegisterDependency(context.Context, identity.Envelope, string, Dependency) error
	DependencyHealth(context.Context, identity.Envelope, Dependency) ([]HealthEvent, error)
}

func (s *Service) profiles() (ProfileRepository, error) {
	repo, ok := s.repo.(ProfileRepository)
	if !ok {
		return nil, ErrUnavailable
	}
	return repo, nil
}

func (s *Service) profileRecord(ctx context.Context, e identity.Envelope, spec ProfileSpec) (ProfileRecord, error) {
	if !spec.Valid() {
		return ProfileRecord{}, ErrInvalid
	}
	if err := spec.Require(e, true); err != nil {
		return ProfileRecord{}, err
	}
	binding, err := s.sources.Binding(ctx, e, spec.Source, spec.Context)
	if err != nil {
		return ProfileRecord{}, err
	}
	if err = readexec.Require(e, binding, []string{spec.Dataset}); err != nil {
		return ProfileRecord{}, err
	}
	policy := config.ProfilePolicy{ID: "redact-default", Tenant: e.Tenant(), Source: spec.Source, RangeColumns: []string{}}
	if spec.Policy != "" {
		found := false
		for _, p := range s.values.Profiling.Policies {
			if p.ID == spec.Policy && p.Tenant == e.Tenant() && p.Source == spec.Source {
				policy = p
				policy.RangeColumns = append([]string(nil), p.RangeColumns...)
				found = true
				break
			}
		}
		if !found {
			return ProfileRecord{}, ErrInvalid
		}
	}
	settings := s.values.Profiling.Clone()
	settings.Policies = []config.ProfilePolicy{}
	r := ProfileRecord{Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Spec: spec, Binding: binding.Clone(), Policy: policy, Settings: settings, State: "reserved", Created: time.Now().UTC().Truncate(time.Microsecond)}
	r.Spec.Columns = append([]string(nil), spec.Columns...)
	r.SpecHash = r.Digest()
	columns, err := selectedColumns(r)
	if err != nil {
		return ProfileRecord{}, err
	}
	for _, c := range columns {
		if c.Name == spec.TimeColumn && !profileTimeType(c.NativeType) {
			return ProfileRecord{}, readexec.ErrUnsupported
		}
	}
	return r, nil
}

// Build creates or resumes one immutable profile version. Explicit resume never
// changes its policy, sample bounds, source binding or skip_llm decision.
func (s *Service) Build(ctx context.Context, e identity.Envelope, spec ProfileSpec, key string, resume bool) (out ProfileRun, err error) {
	// Empty and omitted selections are the same immutable input. Detach caller
	// storage and canonicalize before comparing it with a retained manifest.
	spec.Columns = append([]string(nil), spec.Columns...)
	if !spec.Valid() || !identity.Identifier(key) {
		return out, ErrInvalid
	}
	if err = spec.Require(e, true); err != nil {
		return out, err
	}
	if !s.values.Profiling.Enabled {
		return out, ErrUnavailable
	}
	repo, err := s.profiles()
	if err != nil {
		return out, err
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		r, err := repo.ReadProfile(ctx, e, spec.ID, true)
		if errors.Is(err, store.ErrNotFound) {
			r, err = s.profileRecord(ctx, e, spec)
			if err != nil {
				return err
			}
			r, err = repo.ReserveProfile(ctx, e, r)
		}
		if err != nil {
			return err
		}
		if readexec.Hash(r.Spec) != readexec.Hash(spec) {
			return store.ErrConflict
		}
		if r.State == "complete" {
			out.Profile = r.Public()
			out.Code = "already_complete"
			return nil
		}
		binding, err := s.sources.Binding(ctx, e, r.Spec.Source, r.Spec.Context)
		if err != nil {
			return err
		}
		if readexec.Hash(binding) != readexec.Hash(r.Binding) {
			return readexec.ErrBinding
		}
		if err = readexec.Require(e, binding, []string{spec.Dataset}); err != nil {
			return err
		}
		task, err := s.runner.Admit(ctx, e, key, jobs.RequestInput{Kind: "profile.build", Target: spec.Source, Context: spec.Context, InputHash: r.SpecHash})
		if err != nil {
			return err
		}
		if resume {
			task, err = s.runner.Resume(ctx, e, task.ID)
			if err != nil {
				return err
			}
		}
		r, err = repo.AttachProfile(ctx, e, spec.ID, task)
		if err != nil {
			return err
		}
		task, runErr := s.runner.Run(ctx, e, task, time.Minute, func(ctx context.Context, i jobs.Invocation) error {
			current, err := repo.ReadProfile(ctx, e, spec.ID, true)
			if err != nil {
				return err
			}
			if current.Operation != i.Lease().Task.ID {
				return store.ErrConflict
			}
			var profile Profile
			if current.Result != nil {
				profile = *current.Result
			} else {
				if err = s.previousProfileRead(ctx, e, current); err != nil {
					return err
				}
				operation := "profile-read-" + readexec.Hash([]any{current.SpecHash, i.Lease().Task.ID, i.Lease().Fence})[:40]
				until, _ := ctx.Deadline()
				budget := time.Now().Add(time.Duration(current.Settings.Timeout))
				if budget.Before(until) {
					until = budget
				}
				if err = repo.StartProfileRead(ctx, i, current, operation, until); err != nil {
					return err
				}
				columns, err := selectedColumns(current)
				if err != nil {
					return err
				}
				relation, err := relationFor(current)
				if err != nil {
					return err
				}
				names := make([]string, len(columns))
				for j, c := range columns {
					names[j] = pgx.Identifier{c.Name}.Sanitize()
				}
				// The prefix is not random or representative. No client-supplied expression,
				// filter or SQL correction is introduced by this profiling consumer.
				sql := "SELECT " + strings.Join(names, ",") + " FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize()
				queryCtx, stop := context.WithDeadline(ctx, until)
				defer stop()
				plan, err := s.validator.Validate(queryCtx, e, readexec.Request{Source: spec.Source, Context: spec.Context, SQL: sql, Parameters: []readexec.Parameter{}})
				if err != nil {
					return err
				}
				start := time.Now()
				report, err := s.executor.ExecuteCapped(queryCtx, e, plan, readexec.Options{Operation: operation, Number: 1}, readexec.Caps{Rows: current.Settings.SampleRows, Bytes: current.Settings.SampleBytes, Timeout: time.Duration(current.Settings.Timeout), PlannerCost: current.Settings.PlannerCostCeiling})
				if err != nil {
					return err
				}
				if report.Result == nil {
					switch report.Attempt.Status {
					case "uncertain":
						return ErrState
					case "cancelled":
						return context.Canceled
					case "timed_out":
						return context.DeadlineExceeded
					default:
						return ErrUnavailable
					}
				}
				profile, err = BuildProfile(ctx, current, report, time.Since(start), time.Now().UTC())
				if err != nil {
					return err
				}
				if err = repo.CheckpointProfile(ctx, i, current, profile); err != nil {
					return err
				}
				current.Result = &profile
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if current.Spec.SkipLLM {
				profile.Summary = ProfileSummary{Status: "skipped", Receipt: emptyModelReceipt()}
			} else if !current.Settings.Summaries || s.gateway == nil {
				profile.Summary = ProfileSummary{Status: "disabled", Receipt: emptyModelReceipt()}
			} else {
				started, err := repo.StartProfileSummary(ctx, i, current)
				if err != nil {
					return err
				}
				if started {
					profile.Summary = s.summarize(ctx, e, current, profile)
				} else {
					profile.Summary = ProfileSummary{Status: "unknown_previous_attempt", Receipt: emptyModelReceipt()}
				}
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			return repo.PublishProfile(ctx, i, current, profile)
		})
		if task.ID == "" {
			return runErr
		}
		current, err := repo.ReadProfile(ctx, e, spec.ID, true)
		if err != nil {
			return err
		}
		out = ProfileRun{Profile: current.Public(), Operation: task}
		if runErr != nil {
			out.Code = engineeringCode(runErr)
		}
		return nil
	})
	return out, err
}

func (s *Service) previousProfileRead(ctx context.Context, e identity.Envelope, r ProfileRecord) error {
	if r.LastReadOperation == "" {
		return nil
	}
	attempt, err := s.executor.ByOperation(ctx, e, r.LastReadOperation)
	if errors.Is(err, store.ErrNotFound) {
		if time.Now().Before(r.LastReadDeadline) {
			return ErrState
		}
		return nil
	}
	if err != nil {
		return err
	}
	if attempt.Status == "uncertain" {
		control, e2 := s.executor.Control(ctx, e, attempt.ID, false)
		if e2 != nil {
			return e2
		}
		attempt = control.Attempt
	}
	if attempt.Finished == nil || attempt.RemoteState != "stopped" && attempt.RemoteState != "not_issued" {
		return ErrState
	}
	return nil
}

// InspectProfile needs neither a warehouse credential nor an available model.
func (s *Service) InspectProfile(ctx context.Context, e identity.Envelope, id string) (out ProfileStatus, err error) {
	repo, err := s.profiles()
	if err != nil {
		return out, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		r, e2 := repo.ReadProfile(ctx, e, id, false)
		if e2 == nil {
			out = r.Public()
		}
		return e2
	})
	return out, err
}

// Evidence is a real inspection/planning consumer, never a semantic-publication
// transition and never a reason to regenerate or execute approved SQL.
func (s *Service) Evidence(ctx context.Context, e identity.Envelope, id string) (out ProfileEvidence, err error) {
	repo, err := s.profiles()
	if err != nil {
		return out, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error { var e2 error; out, e2 = repo.ProfileEvidence(ctx, e, id); return e2 })
	return out, err
}

// History is scoped before LIMIT; an old context requires its own signed reach.
func (s *Service) History(ctx context.Context, e identity.Envelope, source, partition, dataset string, limit int) (out []ProfileStatus, err error) {
	repo, err := s.profiles()
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 32 {
		return nil, ErrInvalid
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		var e2 error
		out, e2 = repo.ProfileHistory(ctx, e, source, partition, dataset, limit)
		return e2
	})
	return out, err
}

// RegisterDependency records a consumer's exact version and column dependence.
// Future consumer services must register their own versions through this seam;
// this operation does not assert that a reporting definition was published.
func (s *Service) RegisterDependency(ctx context.Context, e identity.Envelope, profile string, d Dependency) error {
	if !d.Valid() {
		return ErrInvalid
	}
	if err := d.Require(e, true); err != nil {
		return err
	}
	repo, err := s.profiles()
	if err != nil {
		return err
	}
	return s.call(ctx, e, false, func(ctx context.Context) error { return repo.RegisterDependency(ctx, e, profile, d) })
}

// Health exposes immutable drift events under the consumer's current signed reach.
func (s *Service) Health(ctx context.Context, e identity.Envelope, d Dependency) (out []HealthEvent, err error) {
	if !d.Valid() {
		return nil, ErrInvalid
	}
	if err = d.Require(e, false); err != nil {
		return nil, err
	}
	repo, err := s.profiles()
	if err != nil {
		return nil, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error { var e2 error; out, e2 = repo.DependencyHealth(ctx, e, d); return e2 })
	return out, err
}

// ReadStage identifies each native attempt independently of profile versioning.
// It is metadata, never a replay credential.
func ReadStage(task jobs.RequestTask, fence int64) string {
	return "profile-stage-" + task.ID + "-" + strconv.FormatInt(fence, 10)
}
