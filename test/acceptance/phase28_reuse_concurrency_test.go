package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

func phase28ConcurrentReuse(t *testing.T, service *reporting.Runs, e identity.Envelope, block string) {
	t.Helper()
	ctx := context.Background()
	type outcome struct {
		view reporting.RunView
		err  error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			v, err := service.Admit(ctx, e, block, reporting.RunRequest{Key: "p28-concurrent-reuse", ReuseMaxAgeSeconds: 60})
			results <- outcome{v, err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.view.ID != second.view.ID || first.view.ManifestDigest != second.view.ManifestDigest {
		t.Fatal("concurrent admission did not seal one manifest", first.err, second.err)
	}
	other, err := service.Admit(ctx, e, block, reporting.RunRequest{Key: "p28-concurrent-reuse-other", ReuseMaxAgeSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	for _, id := range []string{first.view.ID, other.ID} {
		go func() {
			<-start
			v, runErr := service.Run(ctx, e, id, false)
			results <- outcome{v, runErr}
		}()
	}
	close(start)
	for range 2 {
		result := <-results
		if result.err != nil || result.view.State != "succeeded" || result.view.ReusedFrom == "" || len(result.view.QueryAttempts) != 0 {
			t.Fatal("concurrent retained reuse repeated or lost work", result.view.State, result.err)
		}
	}
}
