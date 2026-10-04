package reporting

import (
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
)

// AuthoringPreparationOperationVersion selects the bounded fresh-key contract.
// Omitted versions retain the original request digest for legacy replay. New
// operations opt in explicitly; an SDK must never upgrade a retained request.
const AuthoringPreparationOperationVersion = "prepare-v1"

var (
	// ErrPreparationContract requires explicit migration for unseen legacy contracts.
	ErrPreparationContract = errors.New("reporting: preparation operation contract required")
	// ErrPreparationOperationExpired rejects keys outside the fresh admission window.
	ErrPreparationOperationExpired = errors.New("reporting: preparation operation outside admission window")
)

// AuthoringPreparationOperationTime decodes only canonical version-one keys.
func AuthoringPreparationOperationTime(operation string) (time.Time, bool) {
	parts := strings.Split(operation, ":")
	if len(parts) != 3 || parts[0] != "prepare" || len(parts[1]) > 12 || len(parts[2]) != 32 {
		return time.Time{}, false
	}
	seconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || seconds < 1 || strconv.FormatInt(seconds, 10) != parts[1] {
		return time.Time{}, false
	}
	raw, err := hex.DecodeString(parts[2])
	if err != nil || hex.EncodeToString(raw) != parts[2] {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

// Only fresh admission uses the temporal contract. Retained custody is looked up
// first, with the same current authority and unchanged original input digest.
func AuthoringPreparationAdmission(in AuthoringPrepareRequest, now time.Time) error {
	at, ok := AuthoringPreparationOperationTime(in.Operation)
	if in.OperationVersion != AuthoringPreparationOperationVersion || !ok {
		return ErrPreparationContract
	}
	if at.Before(now.Add(-5*time.Minute)) || at.After(now.Add(30*time.Second)) {
		return ErrPreparationOperationExpired
	}
	return nil
}

// PreparationSettlement is copied only from verified native custody, or from a
// guarded no-admission transition. It is content-free and never authority.
type PreparationSettlement struct {
	Kind        string    `json:"kind"`
	Attempt     string    `json:"attempt,omitempty"`
	Manifest    string    `json:"manifest,omitempty"`
	Status      string    `json:"status"`
	RemoteState string    `json:"remote_state"`
	Finished    time.Time `json:"finished_at"`
}

// AuthoringPreparationConsumed is the closed compact receipt retained with the
// original native revision. No SQL, request prose, rows or technical catalog is
// copied. Signed reach is enforced before selecting it and again by the service.
type AuthoringPreparationConsumed struct {
	ID              string                `json:"id"`
	Tenant          string                `json:"tenant"`
	Actor           string                `json:"actor"`
	Session         string                `json:"session"`
	Target          string                `json:"target"`
	Operation       string                `json:"operation"`
	InputDigest     string                `json:"input_digest"`
	SourceOperation string                `json:"source_operation"`
	Digest          string                `json:"digest"`
	Source          string                `json:"source"`
	SourceRevision  int64                 `json:"source_revision"`
	Context         string                `json:"context"`
	Dataset         string                `json:"dataset"`
	Topic           TopicPin              `json:"topic"`
	References      []ResourceReference   `json:"references"`
	Revision        int64                 `json:"revision"`
	RevisionDigest  string                `json:"revision_digest"`
	ExecutionDigest string                `json:"execution_digest"`
	ExpiresAt       time.Time             `json:"expires_at"`
	Settlement      PreparationSettlement `json:"settlement"`
	SettledAt       time.Time             `json:"settled_at"`
	ConsumedAt      time.Time             `json:"consumed_at"`
}

func (AuthoringPreparationConsumed) String() string {
	return "authoring-preparation-consumed(redacted)"
}
func (r AuthoringPreparationConsumed) GoString() string { return r.String() }

// PreparationRecord restores only the bounded internal authority/replay coordinates.
func (r AuthoringPreparationConsumed) PreparationRecord() AuthoringPreparationRecord {
	return AuthoringPreparationRecord{ID: r.ID, Actor: r.Actor, Session: r.Session, Target: r.Target, Operation: r.Operation, InputDigest: r.InputDigest, SourceOperation: r.SourceOperation, Digest: r.Digest, Binding: exec.Binding{Tenant: r.Tenant, Source: r.Source, Context: r.Context, Revision: r.SourceRevision}, Topics: []TopicPin{r.Topic}, References: clone(r.References), Request: AuthoringPrepareRequest{NewBlock: r.Target, Operation: r.Operation, Intent: AuthoringDatasetIntent{Topic: r.Topic, Dataset: r.Dataset}}, ExpiresAt: r.ExpiresAt, Status: "consumed", Consumed: clone(&r)}
}
