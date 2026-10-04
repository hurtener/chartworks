package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestReportAppOptionRetention(t *testing.T) {
	f, s, author, prepare, _, scopes := filteredDatasetFixture(t)
	ctx := t.Context()
	metadata := support.Raw(t, f.f.f.dsn)
	in := optionsForPreparation(prepare, 41)
	if page, err := s.DatasetOptions(ctx, author, in); err != nil || !page.ValuesAvailable {
		t.Fatal(page, err)
	}
	base, err := f.f.f.db.ReadAuthoringOption(ctx, author, reporting.AuthoringOptionReference{Target: in.Target, Operation: in.Operation})
	if err != nil {
		t.Fatal(err)
	}
	// Seed synthetic already-aged metadata to exercise actual reservation-time
	// pruning without a day-long wall-clock test. These are not source receipts.
	seed := func(n int, status string) reporting.AuthoringOptionRecord {
		t.Helper()
		r := phase27Copy(t, base)
		r.Operation = "option:" + strconv.FormatInt(time.Now().Add(-25*time.Hour).Unix(), 10) + ":" + fmt.Sprintf("%032x", n)
		r.SourceOperation = "authoring-option:" + readexec.Hash([]string{r.Tenant, r.Actor, r.Session, r.Operation})
		r.CreatedAt = time.Now().Add(-25 * time.Hour).UTC()
		r.Deadline = r.CreatedAt.Add(30 * time.Second)
		r.Status = status
		r.Code = "result_not_retained"
		r.Receipt = nil
		r.ExecutionStatus = ""
		r.RemoteState = "not_issued"
		if status == "uncertain" {
			r.Code = "execution_outcome_unknown"
			r.RemoteState = "unknown"
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := metadata.Exec(ctx, `INSERT INTO chartworks.authoring_option_operations(tenant_id,actor_id,session_id,operation_id,target_digest,namespace_digest,input_digest,source_operation,record,status,created_at,deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.Tenant, r.Actor, r.Session, r.Operation, readexec.Hash(r.Target), reporting.AuthoringOptionNamespace(r.Target), r.InputDigest, r.SourceOperation, raw, r.Status, r.CreatedAt, r.Deadline); err != nil {
			t.Fatal(err)
		}
		return r
	}
	expired := seed(42, "failed")
	unknown := seed(43, "uncertain")
	next := phase27Copy(t, in)
	next.Operation = optionKey(44)
	if _, err := s.DatasetOptions(ctx, author, next); !errors.Is(err, readexec.ErrUncertain) {
		t.Fatal("expired unknown liability discarded", err)
	}
	// An independent target can proceed and perform bounded terminal pruning.
	independent := phase27Copy(t, next)
	independent.Operation = optionKey(45)
	independent.Target.Dataset.NewBlock = "independent-option-chart"
	other := phase27Actor(t, f.f, author.User(), append(slices.Clone(scopes), "cw.block.read:independent-option-chart", "cw.block.write:independent-option-chart", "cw.block.preview:independent-option-chart"))
	if page, err := s.DatasetOptions(ctx, other, independent); err != nil || !page.ValuesAvailable {
		t.Fatal("unrelated target fenced", page, err)
	}
	before := f.attemptCount(t)
	if _, err := s.OptionStatus(ctx, author, reporting.AuthoringOptionReference{Target: expired.Target, Operation: expired.Operation}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("aged terminal metadata not pruned", err)
	}
	oldRequest := phase27Copy(t, in)
	oldRequest.Operation = expired.Operation
	if _, err := s.DatasetOptions(ctx, author, oldRequest); !errors.Is(err, store.ErrExpired) {
		t.Fatal("evicted key became a new operation", err)
	}
	if pending, err := s.OptionStatus(ctx, author, reporting.AuthoringOptionReference{Target: unknown.Target, Operation: unknown.Operation}); err != nil || pending.Status != "uncertain" || pending.NewOperationAllowed {
		t.Fatal("old liability not inspectable", pending, err)
	}
	if ended, err := s.OptionControl(ctx, author, reporting.AuthoringOptionControlRequest{Target: unknown.Target, Operation: unknown.Operation, Action: "reconcile"}); err != nil || ended.Status != "failed" || !ended.NewOperationAllowed || ended.ValuesAvailable {
		t.Fatal("expired never-dispatched operation not reconcilable", ended, err)
	}
	if f.attemptCount(t) != before {
		t.Fatal("metadata recovery repeated source work")
	}
	if fresh, err := s.DatasetOptions(ctx, author, next); err != nil || !fresh.ValuesAvailable || f.attemptCount(t) != before+1 {
		t.Fatal("explicit fresh lookup after reconciliation", fresh, err)
	}
	// Pending records reserve their worst-case sealed metadata, not their
	// currently tiny JSON payload. Unknown records cannot be pruned for room.
	for i := 100; i < 164; i++ {
		seed(i, "uncertain")
	}
	independent.Operation = optionKey(46)
	beforeBudget := f.attemptCount(t)
	if response, err := s.DatasetOptions(ctx, other, independent); !errors.Is(err, readexec.ErrLimit) || response.ValuesAvailable || f.attemptCount(t) != beforeBudget {
		t.Fatal("pending custody bytes were not reserved", response, err)
	}

}
