package nlqbyo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

func opaqueID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(value[:]), nil
}

// ReferenceValid checks only syntax; no caller may use it as authorization.
func ReferenceValid(in Reference) bool {
	b, err := hex.DecodeString(in.ID)
	return in.SchemaVersion == Version && identity.Identifier(in.Context) && err == nil && len(b) == 32 && hex.EncodeToString(b) == in.ID
}

// RecordValid defends the storage seam without manufacturing caller authority.
func RecordValid(r Record, scope store.Scope, limits config.QueryBundles) bool {
	b := r.Bundle
	if !scope.Valid() || !ReferenceValid(b.Reference) || !identity.Identifier(r.Session) || !r.Binding.Valid() || r.Binding.Tenant != scope.Tenant() || r.Binding.Source != b.Source || r.Binding.Context != b.Reference.Context || r.Digest != exec.Hash(b) || !topics.DigestValid(r.DataReach) || b.CreatedAt.IsZero() || !b.ExpiresAt.After(b.CreatedAt) || b.ExpiresAt.Sub(b.CreatedAt) > time.Hour || r.RetainUntil.Before(b.ExpiresAt) || r.RetainUntil.Sub(b.CreatedAt) > 7*24*time.Hour || b.MaxSteps < 1 || b.MaxSteps > limits.MaxSteps || len(b.Semantics) < 1 || len(b.Semantics) > 4 {
		return false
	}
	seen := map[string]bool{}
	for _, pin := range b.Semantics {
		if seen[pin.Topic] || !identity.Identifier(pin.Topic) || !identity.Identifier(pin.Version) || !topics.DigestValid(pin.Digest) || (pin.RuleVersion == "") != (pin.RuleDigest == "") || pin.RuleVersion != "" && (!identity.Identifier(pin.RuleVersion) || !topics.DigestValid(pin.RuleDigest)) {
			return false
		}
		seen[pin.Topic] = true
	}
	raw, err := json.Marshal(r)
	return err == nil && len(raw) <= limits.MaxBytes
}

// dataReach pins data-reading reach, not operation permissions. A separately
// granted query.submit/sources.query action and cw.source.query reach may enable
// submission, but changing topic/dataset/partition reach requires new context.
func dataReach(e identity.Envelope) string {
	var values []string
	for _, r := range e.Reach() {
		if r.Kind == "topic" && r.Permission == "read" || r.Kind == "dataset" && r.Permission == "query" || r.Kind == "source" && r.Permission == "read" || r.Kind == "execution_context" && r.Permission == "use" {
			values = append(values, r.Kind+"."+r.Permission+":"+r.ID)
		}
	}
	sort.Strings(values)
	return exec.Hash(values)
}

func replanError(err error) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) || errors.Is(err, exec.ErrBinding) || errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
		return ErrReplan
	}
	return err
}

func parameterStyle(dialect string) string {
	switch dialect {
	case "postgres":
		return "$1, $2, ..."
	case "sqlserver", "bigquery":
		return "@p1, @p2, ..."
	case "databricks":
		return ":p1, :p2, ..."
	default:
		return "? (positional)"
	}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, exec.ErrUnsafe):
		return "sql_unsafe"
	case errors.Is(err, exec.ErrUnsupported), errors.Is(err, exec.ErrType):
		return "unsupported"
	case errors.Is(err, exec.ErrLimit):
		return "limit_exceeded"
	case errors.Is(err, exec.ErrBinding), errors.Is(err, ErrReplan):
		return "replan_required"
	case errors.Is(err, access.ErrForbidden), errors.Is(err, access.ErrUnauthenticated):
		return "forbidden"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, exec.ErrCancelled), errors.Is(err, exec.ErrTimeout):
		return "cancelled_or_timed_out"
	default:
		return "execution_outcome_unknown"
	}
}

// StepValid restricts receipt payloads to the fixed content-free durable contract.
func StepValid(s Step) bool {
	if !identity.Identifier(s.Operation) || s.Number < 0 || s.Number > 32 || !topics.DigestValid(s.InputDigest) || !topics.DigestValid(s.BundleDigest) || len(s.Semantics) < 1 || len(s.Semantics) > 4 || s.ModelCalls != 0 || s.CreatedAt.IsZero() || !s.Deadline.After(s.CreatedAt) || s.Deadline.Sub(s.CreatedAt) > time.Minute {
		return false
	}
	switch s.Status {
	case "accepted", "rejected", "succeeded", "empty", "truncated", "failed", "uncertain", "cancelled", "timed_out", "interrupted":
	default:
		return false
	}
	if !identity.Identifier(s.Code) || len(s.Code) > 64 || s.FinishedAt != nil && s.FinishedAt.Before(s.CreatedAt) {
		return false
	}
	seen := map[string]bool{}
	for _, p := range s.Semantics {
		if seen[p.Topic] || !identity.Identifier(p.Topic) || !identity.Identifier(p.Version) || !topics.DigestValid(p.Digest) || (p.RuleVersion == "") != (p.RuleDigest == "") || p.RuleVersion != "" && (!identity.Identifier(p.RuleVersion) || !topics.DigestValid(p.RuleDigest)) {
			return false
		}
		seen[p.Topic] = true
	}
	raw, err := json.Marshal(s)
	return err == nil && len(raw) <= 32<<10
}
