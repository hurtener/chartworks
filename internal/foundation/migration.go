package foundation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
)

type migrationDomains struct {
	sources     *sources.Service
	engineering *engineering.Service
	topics      *drafts.Service
	rules       *rulesets.Service
	blocks      *reporting.Service
	documents   *reporting.Documents
	queries     *nlqexec.Service
	schedules   *jobs.Service
}

type scheduleImport struct {
	Key     string               `json:"key"`
	Request jobs.ScheduleRequest `json:"request"`
}

func newMigrationService(db *postgres.DB, d migrationDomains) (*migration.Service, error) {
	refs := migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, m migration.Mapping) error {
		if m.Destination == "" && o.Kind != migration.KindCalibration {
			return migration.ErrUnsupported
		}
		return nil
	}, ApplyFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, m migration.Mapping, _ string) (string, error) {
		if o.Kind == migration.KindCalibration {
			return "review-candidate:" + o.ExternalRef, nil
		}
		return m.Destination, nil
	}}
	adapters := map[migration.Kind]migration.Adapter{
		migration.KindUpload: refs, migration.KindProfile: refs, migration.KindFilter: refs, migration.KindCalibration: refs,
		migration.KindTombstone: migration.AdapterFuncs{ValidateFunc: func(context.Context, identity.Envelope, migration.Object, migration.Mapping) error { return nil }, ApplyFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping, _ string) (string, error) {
			return "tombstone:" + o.ExternalRef, nil
		}},
	}
	if d.sources != nil {
		adapters[migration.KindSource] = migration.AdapterFuncs{ValidateFunc: func(ctx context.Context, e identity.Envelope, _ migration.Object, m migration.Mapping) error {
			if m.Destination == "" {
				return migration.ErrUnsupported
			}
			_, err := d.sources.Get(ctx, e, m.Destination)
			return err
		}, ApplyFunc: func(_ context.Context, _ identity.Envelope, _ migration.Object, m migration.Mapping, _ string) (string, error) {
			return m.Destination, nil
		}}
	}
	if d.engineering != nil {
		adapters[migration.KindUpload] = migration.AdapterFuncs{ValidateFunc: func(ctx context.Context, e identity.Envelope, _ migration.Object, m migration.Mapping) error {
			if m.Destination == "" {
				return migration.ErrUnsupported
			}
			_, err := d.engineering.InspectUpload(ctx, e, m.Destination)
			return err
		}, ApplyFunc: func(_ context.Context, _ identity.Envelope, _ migration.Object, m migration.Mapping, _ string) (string, error) {
			return m.Destination, nil
		}}
		adapters[migration.KindProfile] = migration.AdapterFuncs{ValidateFunc: func(ctx context.Context, e identity.Envelope, _ migration.Object, m migration.Mapping) error {
			if m.Destination == "" {
				return migration.ErrUnsupported
			}
			_, err := d.engineering.InspectProfile(ctx, e, m.Destination)
			return err
		}, ApplyFunc: func(_ context.Context, _ identity.Envelope, _ migration.Object, m migration.Mapping, _ string) (string, error) {
			return m.Destination, nil
		}}
	}
	if d.topics != nil {
		adapters[migration.KindTopic] = migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping) error {
			var in drafts.ImportRequest
			if err := decodeMigrationPayload(o.Payload, &in); err != nil {
				return err
			}
			_, err := semantics.ImportDraftCandidate(in.Portable, in.Bindings)
			return err
		}, ApplyFunc: func(ctx context.Context, e identity.Envelope, o migration.Object, _ migration.Mapping, _ string) (string, error) {
			var in drafts.ImportRequest
			if err := decodeMigrationPayload(o.Payload, &in); err != nil {
				return "", err
			}
			candidate, err := semantics.ImportDraftCandidate(in.Portable, in.Bindings)
			if err != nil {
				return "", err
			}
			v, err := d.topics.Import(ctx, e, in)
			if errors.Is(err, store.ErrConflict) {
				v, err = d.topics.Read(ctx, e, in.Bindings.Topic, in.Expected+1)
				if err == nil && v.Metadata.Digest != candidate.Digest() {
					err = store.ErrConflict
				}
			}
			if err != nil {
				return "", err
			}
			return v.Metadata.Version, nil
		}}
	}
	if d.rules != nil {
		adapters[migration.KindRule] = migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping) error {
			var in rulesets.SaveRequest
			return decodeMigrationPayload(o.Payload, &in)
		}, ApplyFunc: func(ctx context.Context, e identity.Envelope, o migration.Object, _ migration.Mapping, _ string) (string, error) {
			var in rulesets.SaveRequest
			if err := decodeMigrationPayload(o.Payload, &in); err != nil {
				return "", err
			}
			v, err := d.rules.Save(ctx, e, in)
			if errors.Is(err, store.ErrConflict) {
				v, err = d.rules.ReconcileDraft(ctx, e, in)
			}
			if err != nil {
				return "", err
			}
			return v.Topic + ":draft:" + strconv.FormatInt(v.Revision, 10), nil
		}}
	}
	if d.blocks != nil {
		adapters[migration.KindBlock] = migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping) error {
			var in reporting.CreateRequest
			return decodeMigrationPayload(o.Payload, &in)
		}, ApplyFunc: func(ctx context.Context, e identity.Envelope, o migration.Object, m migration.Mapping, _ string) (string, error) {
			var in reporting.CreateRequest
			if err := decodeMigrationPayload(o.Payload, &in); err != nil {
				return "", err
			}
			if m.Destination != "" {
				in.ID = m.Destination
			}
			v, err := d.blocks.Create(ctx, e, in)
			if errors.Is(err, store.ErrConflict) {
				v, err = d.blocks.Read(ctx, e, in.ID, reporting.Reference{Draft: true})
				if err == nil && v.Digest != reporting.DefinitionDigest(in.Definition) {
					err = store.ErrConflict
				}
			}
			if err != nil {
				return "", err
			}
			return v.State.ID + ":draft:" + strconv.FormatInt(v.State.Version, 10), nil
		}}
	}
	if d.documents != nil {
		for _, kind := range []migration.Kind{migration.KindReport, migration.KindDashboard} {
			documentKind := kind
			adapters[kind] = migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping) error {
				if !json.Valid([]byte(o.Payload)) {
					return migration.ErrInvalid
				}
				return nil
			}, ApplyFunc: func(ctx context.Context, e identity.Envelope, o migration.Object, m migration.Mapping, _ string) (string, error) {
				id := m.Destination
				if id == "" {
					id = o.ExternalRef
				}
				out, err := d.documents.Import(ctx, e, string(documentKind), id, json.RawMessage(o.Payload), reporting.ExternalReference{System: o.Origin, ID: o.ExternalRef, Version: strconv.FormatInt(o.Revision, 10)})
				if err != nil {
					return "", err
				}
				if out.State != nil {
					return id + ":draft:" + strconv.FormatInt(out.State.LatestRevision, 10), nil
				}
				return "quarantine:" + out.Quarantine, nil
			}}
		}
	}
	if d.queries != nil {
		adapters[migration.KindTemplate] = migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping) error {
			var in nlqexec.ExampleImportRequest
			return decodeMigrationPayload(o.Payload, &in)
		}, ApplyFunc: func(ctx context.Context, e identity.Envelope, o migration.Object, _ migration.Mapping, _ string) (string, error) {
			var in nlqexec.ExampleImportRequest
			if err := decodeMigrationPayload(o.Payload, &in); err != nil {
				return "", err
			}
			v, err := d.queries.ImportExample(ctx, e, in)
			if err != nil {
				return "", err
			}
			return v.ID + ":candidate", nil
		}}
	}
	if d.schedules != nil {
		adapters[migration.KindSchedule] = migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping) error {
			var in scheduleImport
			if err := decodeMigrationPayload(o.Payload, &in); err != nil {
				return err
			}
			return in.Request.Validate()
		}, ApplyFunc: func(ctx context.Context, e identity.Envelope, o migration.Object, _ migration.Mapping, digest string) (string, error) {
			var in scheduleImport
			if err := decodeMigrationPayload(o.Payload, &in); err != nil {
				return "", err
			}
			if in.Key == "" {
				in.Key = "mig-" + digest[:24]
			}
			v, err := d.schedules.CreateSchedule(ctx, e, in.Key, in.Request)
			if err != nil {
				return "", err
			}
			return v.ID + ":v" + strconv.FormatInt(v.Revision, 10), nil
		}}
	}
	return migration.New(db, adapters, nil)
}

func decodeMigrationPayload(raw string, out any) error {
	if len(raw) == 0 {
		return migration.ErrInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return migration.ErrInvalid
	}
	return nil
}
