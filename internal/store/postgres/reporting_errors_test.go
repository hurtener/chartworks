package postgres

import (
	"errors"
	"fmt"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"testing"
)

func TestReportingErrorsRetainPublicClass(t *testing.T) {
	for _, sentinel := range []error{jobs.ErrReportingBudget, jobs.ErrReportingAttention, jobs.ErrTransient} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			for _, input := range []error{sentinel, fmt.Errorf("sensitive fixture: %w", sentinel), errors.Join(errors.New("private driver detail"), sentinel)} {
				got := safe(input)
				if got != sentinel {
					t.Fatalf("public error class lost: %v", got)
				}
			}
		})
	}
	if got := safe(errors.New("unknown private SQL or token detail")); got != store.ErrUnavailable {
		t.Fatal("unknown storage error leaked", got)
	}
}
