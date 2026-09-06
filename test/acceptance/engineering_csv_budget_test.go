package acceptance

import (
	"context"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/test/support"
)

func TestCSVReservationCoversNormalizationGrowth(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw := []byte("value\n" + strings.Repeat("1e7\n", 16))
	spec := engineeringSpec("compact-csv", "csv", raw, []engineering.UploadColumn{{Name: "value", Type: "number"}})
	parsed, err := engineering.Parse(ctx, raw, spec, f.values.Uploads, func([]engineering.Cell) error { return nil })
	if err != nil || parsed.Rows != 16 || parsed.DecodedBytes <= spec.Bytes {
		t.Fatal("fixture did not exercise normalized-data growth", err, parsed, spec.Bytes)
	}
	f.stage(t, spec, raw)
	metadata := support.Raw(t, f.dsn)
	var accounted int64
	if err = metadata.QueryRow(ctx, `SELECT accounted_bytes FROM chartworks.uploads WHERE tenant_id=$1 AND source_id=$2`, f.e.Tenant(), spec.ID).Scan(&accounted); err != nil || accounted != f.values.Uploads.MaxExpandedBytes {
		t.Fatal("CSV did not reserve its decoded-data ceiling before load", err, accounted)
	}
	loaded, err := f.service.LoadUpload(ctx, f.e, spec.ID, "compact-csv-load", false)
	if err != nil || loaded.Upload.State != "active" || loaded.Upload.Rows != 16 || loaded.Upload.Source == nil {
		t.Fatal("valid normalized CSV was rejected at activation", err, loaded)
	}
	if err = metadata.QueryRow(ctx, `SELECT accounted_bytes FROM chartworks.uploads WHERE tenant_id=$1 AND source_id=$2`, f.e.Tenant(), spec.ID).Scan(&accounted); err != nil || accounted != parsed.DecodedBytes {
		t.Fatal("activation did not release the unused reservation atomically", err, accounted)
	}
	f.readUpload(t, *loaded.Upload.Source)
}
