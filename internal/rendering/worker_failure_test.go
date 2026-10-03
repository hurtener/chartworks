package rendering

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestWorkerFailureClassificationStaysClosed(t *testing.T) {
	for _, tc := range []struct {
		err              error
		diagnostic, code string
	}{
		{fmt.Errorf("private path: %w", os.ErrPermission), "PRIVATE_INPUT", "launch_denied"},
		{errors.New("exit2 PRIVATE_INPUT"), "fatal error: runtime: cannot allocate memory\nPRIVATE_INPUT /private/path", "memory_budget"},
		{errors.New("private process error"), "fatal error: out of memory\nPRIVATE_INPUT", "memory_budget"},
		{errors.New("exit1"), "PRIVATE_INPUT\nfatal error: runtime: cannot allocate memory\n", "process_exit"},
		{errors.New("exit1"), "malformed private payload PRIVATE_INPUT", "process_exit"},
	} {
		got := workerProcessError(tc.err, []byte(tc.diagnostic))
		if !errors.Is(got, ErrWorker) || got.Error() != ErrWorker.Error()+": "+tc.code || strings.Contains(got.Error(), "PRIVATE_INPUT") || strings.Contains(got.Error(), "/private") {
			t.Fatal("worker details leaked or class lost", got)
		}
	}
	if workerProcessError(nil, []byte("PRIVATE_INPUT")) != nil {
		t.Fatal("success became failure")
	}
}
