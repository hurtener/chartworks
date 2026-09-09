package chartservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestInvalidOptionsAndNilAdmission(t *testing.T) {
	good := Options{Limits: charts.Defaults(), MaxConcurrent: 2, Timeout: time.Second, RankCalls: 1, RankTokens: 2048, RankTimeout: time.Second}
	for _, edit := range []func(*Options){func(o *Options) { o.MaxConcurrent = 0 }, func(o *Options) { o.MaxConcurrent = 65 }, func(o *Options) { o.Timeout = 0 }, func(o *Options) { o.Timeout = 31 * time.Second }, func(o *Options) { o.RankTimeout = 2 * time.Second }, func(o *Options) { o.RankTimeout = 0 }, func(o *Options) { o.RankTokens = 1 }, func(o *Options) { o.RankTokens = 65537 }, func(o *Options) { o.RankCalls = 0 }, func(o *Options) { o.RankCalls = 5 }, func(o *Options) { o.Limits.MaxRows = 0 }} {
		bad := good
		edit(&bad)
		if _, err := New(bad, nil); !errors.Is(err, charts.ErrInvalid) {
			t.Fatal("invalid options admitted", err)
		}
	}
	s, err := New(good, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []*Service{nil, s} {
		if _, _, err = service.begin(nil, identity.Envelope{}, "charts.read"); !errors.Is(err, charts.ErrInvalid) { //nolint:staticcheck // Deliberately probe rejection of a missing context.
			t.Fatal("nil context accepted", err)
		}
	}
	e := identity.Envelope{}
	if _, err = s.Catalog(context.Background(), e); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = s.Build(context.Background(), e, BuildRequest{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = s.Specify(context.Background(), e, SpecifyRequest{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err = s.Rebind(context.Background(), e, BuildRequest{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
}
func TestReceiptCopiesPointerObservations(t *testing.T) {
	in, out, cost := 10, 5, .002
	r := gateway.Receipt{Warning: "rank_unavailable", Calls: []gateway.Usage{{InputTokens: &in, OutputTokens: &out, CostUSD: &cost}}}
	copy := copyReceipt(r)
	*copy.Calls[0].InputTokens = 99
	*copy.Calls[0].OutputTokens = 88
	*copy.Calls[0].CostUSD = 1
	if in != 10 || out != 5 || cost != .002 || copy.Warning != r.Warning {
		t.Fatal("shared mutable receipt")
	}
	f := &Failure{Cause: context.Canceled, Receipt: r}
	if !errors.Is(f, context.Canceled) || f.Error() != "charts: selection interrupted" {
		t.Fatal("failure contract")
	}
}
