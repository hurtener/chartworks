package onboarding

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// GoalService composes existing authorized catalogs. Its lexical policy has no
// remote model calls, reads no warehouse rows, and never infers approved meaning.
type GoalService struct {
	sources interface {
		List(context.Context, identity.Envelope, int) ([]sources.Source, error)
		ListDatasets(context.Context, identity.Envelope, sources.DatasetListRequest) ([]sources.Dataset, error)
		DescribeDataset(context.Context, identity.Envelope, sources.DatasetDescribeRequest) (sources.Dataset, error)
	}
	profiles interface {
		ActiveProfile(context.Context, identity.Envelope, string, string, string) (engineering.ProfileStatus, error)
		Evidence(context.Context, identity.Envelope, string) (engineering.ProfileEvidence, error)
	}
	topics interface {
		List(context.Context, identity.Envelope, topics.ListRequest) ([]topics.Summary, error)
		Contract(context.Context, identity.Envelope, string) (topics.Contract, error)
	}
	drafts interface {
		OnboardProfile(context.Context, identity.Envelope, drafts.OnboardRequest) (drafts.Version, error)
	}
}

func NewGoalService(s *sources.Service, p *engineering.Service, t *topics.Service, d *drafts.Service) (*GoalService, error) {
	if s == nil || p == nil || t == nil || d == nil {
		return nil, ErrInvalid
	}
	return &GoalService{sources: s, profiles: p, topics: t, drafts: d}, nil
}

type GoalSearchRequest struct {
	Goal   string `json:"goal"`
	Locale string `json:"locale" jsonschema:"enum=en,enum=es"`
}

type GoalCandidate struct {
	Kind           string           `json:"kind" jsonschema:"enum=topic,enum=profile,enum=dataset"`
	Topic          string           `json:"topic,omitempty"`
	Version        string           `json:"version,omitempty"`
	Revision       int64            `json:"revision,omitempty"`
	Digest         string           `json:"digest,omitempty"`
	Source         string           `json:"source,omitempty"`
	Context        string           `json:"context,omitempty"`
	Contexts       []string         `json:"contexts,omitempty"`
	Bindings       []topics.Binding `json:"bindings,omitempty"`
	Dataset        string           `json:"dataset,omitempty"`
	Profile        string           `json:"profile,omitempty"`
	SourceRevision int64            `json:"source_revision,omitempty"`
	Score          int              `json:"score"`
	MatchedTerms   []string         `json:"matched_terms"`
	Evidence       []string         `json:"evidence"`
}

type GoalSearchResult struct {
	Policy     string          `json:"policy"`
	Candidates []GoalCandidate `json:"candidates"`
	Unresolved []string        `json:"unresolved"`
	Truncated  bool            `json:"truncated"`
}

type GoalChoiceRequest struct {
	Kind           string `json:"kind" jsonschema:"enum=reuse,enum=new_draft"`
	Topic          string `json:"topic"`
	Version        string `json:"version"`
	Revision       int64  `json:"revision,omitempty"`
	Digest         string `json:"digest,omitempty"`
	Source         string `json:"source,omitempty"`
	Context        string `json:"context"`
	Dataset        string `json:"dataset,omitempty"`
	Profile        string `json:"profile,omitempty"`
	SourceRevision int64  `json:"source_revision,omitempty"`
	Name           string `json:"name,omitempty"`
	Description    string `json:"description,omitempty"`
}

type GoalChoiceResult struct {
	Kind       string          `json:"kind"`
	Topic      string          `json:"topic"`
	Version    string          `json:"version"`
	Revision   int64           `json:"revision"`
	Digest     string          `json:"digest"`
	Draft      *drafts.Version `json:"draft,omitempty"`
	NextAction string          `json:"next_action"`
}

func goalTerms(goal string) []string {
	seen := map[string]bool{}
	var out []string
	for _, term := range strings.FieldsFunc(strings.ToLower(goal), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if utf8.RuneCountInString(term) < 3 || seen[term] {
			continue
		}
		seen[term] = true
		out = append(out, term)
		if len(out) == 24 {
			break
		}
	}
	return out
}

func goalMatch(terms []string, text string) (int, []string) {
	text = strings.ToLower(text)
	var matched []string
	for _, term := range terms {
		if strings.Contains(text, term) {
			matched = append(matched, term)
		}
	}
	return len(matched), matched
}

// Search admits signed topic and source/dataset/context scope in their owning
// catalog stores before scoring. Caps are fail-visible and no broad catalog is
// fetched for client-side filtering.
func (s *Service) SearchGoal(ctx context.Context, e identity.Envelope, in GoalSearchRequest) (GoalSearchResult, error) {
	if s.goal == nil || ctx == nil || in.Locale != "en" && in.Locale != "es" || !validText(in.Goal, 2048) {
		return GoalSearchResult{}, ErrInvalid
	}
	if err := require(e, "onboarding.read", "", "read"); err != nil {
		return GoalSearchResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	terms := goalTerms(in.Goal)
	if len(terms) == 0 {
		return GoalSearchResult{}, ErrInvalid
	}
	result := GoalSearchResult{Policy: "authorized-lexical-v1", Candidates: []GoalCandidate{}, Unresolved: []string{"business_meaning_requires_human_review"}}
	const maxSources, maxTopics, maxDatasets, maxCandidates = 8, 16, 16, 24
	ss, err := s.goal.sources.List(ctx, e, maxSources+1)
	if err != nil {
		return GoalSearchResult{}, err
	}
	if len(ss) > maxSources {
		result.Truncated = true
		ss = ss[:maxSources]
	}
	for _, source := range ss {
		datasets, err := s.goal.sources.ListDatasets(ctx, e, sources.DatasetListRequest{Source: source.ID, Context: source.ContextID, Limit: maxDatasets + 1})
		if err != nil {
			if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
				continue
			}
			return GoalSearchResult{}, err
		}
		if len(datasets) > maxDatasets {
			result.Truncated = true
			datasets = datasets[:maxDatasets]
		}
		for _, dataset := range datasets {
			label := source.Name + " " + dataset.Relation.Schema + " " + dataset.Relation.Name
			for _, column := range dataset.Relation.Columns {
				if column.Safe {
					label += " " + column.Name
				}
			}
			score, matches := goalMatch(terms, label)
			candidate := GoalCandidate{Kind: "dataset", Source: source.ID, Context: source.ContextID, Dataset: dataset.Relation.ID, SourceRevision: source.Revision, Score: score, MatchedTerms: matches, Evidence: []string{"authorized_registered_dataset", "current_source_context"}}
			result.Candidates = append(result.Candidates, candidate)
			active, err := s.goal.profiles.ActiveProfile(ctx, e, source.ID, source.ContextID, dataset.Relation.ID)
			if err != nil {
				if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) || errors.Is(err, store.ErrNotFound) {
					continue
				}
				return GoalSearchResult{}, err
			}
			if active.Profile != nil && active.Profile.SourceRevision == source.Revision {
				profile := candidate
				profile.Kind, profile.Profile = "profile", active.Version
				profile.Evidence = []string{"active_private_profile", "current_source_revision"}
				result.Candidates = append(result.Candidates, profile)
			}
		}
	}
	published, err := s.goal.topics.List(ctx, e, topics.ListRequest{Limit: maxTopics + 1})
	if err != nil {
		return GoalSearchResult{}, err
	}
	if len(published) > maxTopics {
		result.Truncated = true
		published = published[:maxTopics]
	}
	for _, summary := range published {
		contract, err := s.goal.topics.Contract(ctx, e, summary.Topic)
		if err != nil {
			return GoalSearchResult{}, err
		}
		p := contract.Publication
		if p.State.Revision != summary.Revision || p.Digest != summary.Digest {
			return GoalSearchResult{}, store.ErrConflict
		}
		label := p.Definition.Name + " " + p.Definition.Description
		for _, d := range p.Definition.Datasets {
			label += " " + d.Name
			for _, c := range d.Columns {
				label += " " + c.Name
			}
		}
		score, matches := goalMatch(terms, label)
		contexts := []string{}
		bindings := []topics.Binding{}
		for _, d := range p.Definition.Datasets {
			bindings = append(bindings, d.Source)
			seen := false
			for _, existing := range contexts {
				if existing == d.Source.Context {
					seen = true
					break
				}
			}
			if !seen {
				contexts = append(contexts, d.Source.Context)
			}
		}
		sort.Strings(contexts)
		result.Candidates = append(result.Candidates, GoalCandidate{Kind: "topic", Topic: summary.Topic, Version: summary.Version, Revision: summary.Revision, Digest: summary.Digest, Contexts: contexts, Bindings: bindings, Score: score, MatchedTerms: matches, Evidence: []string{"current_reviewed_publication", "live_source_contract"}})
	}
	sort.SliceStable(result.Candidates, func(i, j int) bool {
		a, b := result.Candidates[i], result.Candidates[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Topic+a.Source+a.Dataset < b.Topic+b.Source+b.Dataset
	})
	if len(result.Candidates) > maxCandidates {
		result.Truncated = true
		result.Candidates = result.Candidates[:maxCandidates]
	}
	return result, nil
}

// ChooseGoal rechecks exact current evidence. Reuse returns a reviewed topic
// reference; new authoring writes only an unresolved actor-private draft.
func (s *Service) ChooseGoal(ctx context.Context, e identity.Envelope, in GoalChoiceRequest) (GoalChoiceResult, error) {
	if s.goal == nil || ctx == nil || !identity.Identifier(in.Topic) || !identity.Identifier(in.Version) || !identity.Identifier(in.Context) {
		return GoalChoiceResult{}, ErrInvalid
	}
	if err := require(e, "onboarding.write", "", "write"); err != nil {
		return GoalChoiceResult{}, err
	}
	switch in.Kind {
	case "reuse":
		if in.Revision < 1 || !topics.DigestValid(in.Digest) || in.Source != "" || in.Dataset != "" || in.Profile != "" || in.SourceRevision != 0 || in.Name != "" || in.Description != "" {
			return GoalChoiceResult{}, ErrInvalid
		}
		contract, err := s.goal.topics.Contract(ctx, e, in.Topic)
		if err != nil {
			return GoalChoiceResult{}, err
		}
		p := contract.Publication
		if p.State.Version != in.Version || p.State.Revision != in.Revision || p.Digest != in.Digest {
			return GoalChoiceResult{}, store.ErrConflict
		}
		found := false
		for _, d := range p.Definition.Datasets {
			if d.Source.Context == in.Context {
				found = true
			}
		}
		if !found {
			return GoalChoiceResult{}, store.ErrConflict
		}
		return GoalChoiceResult{Kind: "reuse", Topic: in.Topic, Version: in.Version, Revision: in.Revision, Digest: in.Digest, NextAction: "use_reviewed_topic"}, nil
	case "new_draft":
		if !identity.Identifier(in.Source) || !identity.Identifier(in.Dataset) || !identity.Identifier(in.Profile) || in.SourceRevision < 1 || in.Revision != 0 || in.Digest != "" || !validText(in.Name, 256) || len(in.Description) > 4096 || !utf8.ValidString(in.Description) {
			return GoalChoiceResult{}, ErrInvalid
		}
		dataset, err := s.goal.sources.DescribeDataset(ctx, e, sources.DatasetDescribeRequest{Source: in.Source, Context: in.Context, Dataset: in.Dataset})
		if err != nil {
			return GoalChoiceResult{}, err
		}
		if dataset.Revision != in.SourceRevision {
			return GoalChoiceResult{}, store.ErrConflict
		}
		profile, err := s.goal.profiles.Evidence(ctx, e, in.Profile)
		if err != nil {
			return GoalChoiceResult{}, err
		}
		if !profile.Active || profile.Profile.Source != in.Source || profile.Profile.Context != in.Context || profile.Profile.Dataset != in.Dataset || profile.Profile.SourceRevision != in.SourceRevision {
			return GoalChoiceResult{}, store.ErrConflict
		}
		v, err := s.goal.drafts.OnboardProfile(ctx, e, drafts.OnboardRequest{Topic: in.Topic, Version: in.Version, Name: in.Name, Description: in.Description, Profile: in.Profile, Change: "Business goal private scaffold for explicit review"})
		if err != nil {
			return GoalChoiceResult{}, err
		}
		return GoalChoiceResult{Kind: "new_draft", Topic: in.Topic, Version: in.Version, Revision: v.Metadata.Revision, Digest: v.Metadata.Digest, Draft: &v, NextAction: "review_business_meaning"}, nil
	default:
		return GoalChoiceResult{}, ErrInvalid
	}
}
