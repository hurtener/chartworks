package charts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
)

func TestSavedBindingListIsBoundedBeforeProjection(t *testing.T) {
	d, m := bind(t, charts.Table)
	limits := charts.Defaults()
	m.Bindings.Columns = make([]string, limits.MaxColumns+1)
	ctx := context.Background()
	if err := charts.ValidateMapping(ctx, d, m, limits); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("saved mapping binding list was not capped before projection", err)
	}
	if _, err := charts.Build(ctx, d, m, limits); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("saved build did not enforce binding allocation cap", err)
	}
	if _, err := charts.Rebind(ctx, d, m, limits); !errors.Is(err, charts.ErrLimit) {
		t.Fatal("authoring rebind did not enforce binding allocation cap", err)
	}
}
