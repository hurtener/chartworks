package reporting

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAuthoringPreparationAdmissionContract(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	key := func(at time.Time) string {
		return "prepare:" + strconv.FormatInt(at.Unix(), 10) + ":" + strings.Repeat("a", 32)
	}
	for _, delta := range []time.Duration{-5 * time.Minute, 0, 30 * time.Second} {
		in := AuthoringPrepareRequest{OperationVersion: AuthoringPreparationOperationVersion, Operation: key(now.Add(delta))}
		if err := AuthoringPreparationAdmission(in, now); err != nil {
			t.Fatal(delta, err)
		}
	}
	for _, delta := range []time.Duration{-5*time.Minute - time.Second, 31 * time.Second} {
		if err := AuthoringPreparationAdmission(AuthoringPrepareRequest{OperationVersion: AuthoringPreparationOperationVersion, Operation: key(now.Add(delta))}, now); !errors.Is(err, ErrPreparationOperationExpired) {
			t.Fatal(delta, err)
		}
	}
	for _, operation := range []string{"legacy", "prepare:01800000000:" + strings.Repeat("a", 32), "prepare:1800000000:" + strings.Repeat("A", 32), "prepare:0:" + strings.Repeat("a", 32), "prepare:1000000000000:" + strings.Repeat("a", 32), "prepare:1800000000:short"} {
		if err := AuthoringPreparationAdmission(AuthoringPrepareRequest{OperationVersion: AuthoringPreparationOperationVersion, Operation: operation}, now); !errors.Is(err, ErrPreparationContract) {
			t.Fatal(operation, err)
		}
	}
	for _, version := range []string{"", "prepare-v2"} {
		if err := AuthoringPreparationAdmission(AuthoringPrepareRequest{OperationVersion: version, Operation: key(now)}, now); !errors.Is(err, ErrPreparationContract) {
			t.Fatal(version, err)
		}
	}
	// The inclusive five-minute endpoint must not qualify for eviction.
	in := AuthoringPrepareRequest{OperationVersion: AuthoringPreparationOperationVersion, Operation: key(now.Add(-5 * time.Minute))}
	if err := AuthoringPreparationAdmission(in, now.Add(time.Nanosecond)); !errors.Is(err, ErrPreparationOperationExpired) {
		t.Fatal(err)
	}
}

func TestAuthoringPreparationLegacyDigestSerialization(t *testing.T) {
	in := AuthoringPrepareRequest{Operation: "legacy"}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "operation_version") {
		t.Fatal("legacy digest changed")
	}
	var restored AuthoringPrepareRequest
	if err := json.Unmarshal(raw, &restored); err != nil || digest(restored) != digest(in) {
		t.Fatal(err)
	}
}

func TestAuthoringPreparationUnknownContractDoesNotExecute(t *testing.T) {
	s, repo, executor, e, in := preparationServiceFixture(t)
	_ = repo
	before := executor.physical.Load()
	in.OperationVersion = ""
	if _, err := s.PrepareDatasetChart(t.Context(), e, in); !errors.Is(err, ErrPreparationContract) {
		t.Fatal(err)
	}
	if executor.physical.Load() != before {
		t.Fatal("legacy fresh contract executed")
	}
}
