package releaseprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/jackc/pgx/v5"
)

type nativeDatasetReader interface {
	DescribeDataset(context.Context, identity.Envelope, sources.DatasetDescribeRequest) (sources.Dataset, error)
	Read(context.Context, identity.Envelope, readexec.Plan) (sources.Rows, error)
}

// NativeDatasetProbe observes all rows of one bounded, registered PostgreSQL
// dataset's validator-safe column projection through the consumer read path. A
// truncated read never yields a release digest. Multi-dataset source snapshots
// need a shared native transaction, so this seam fails closed for that case.
type NativeDatasetProbe struct {
	validator *readexec.Validator
	source    nativeDatasetReader
}

func NewNativeDatasetProbe(validator *readexec.Validator, source nativeDatasetReader) (*NativeDatasetProbe, error) {
	if validator == nil || source == nil {
		return nil, readexec.ErrBinding
	}
	return &NativeDatasetProbe{validator: validator, source: source}, nil
}

func (p *NativeDatasetProbe) ObserveDataset(ctx context.Context, e identity.Envelope, source sources.Source, datasets []string) (DatasetEvidence, error) {
	if p == nil || ctx == nil || !e.Valid() || source.Dialect != "postgres" || !identity.Identifier(source.ID) || !identity.Identifier(source.ContextID) || source.Revision < 1 || len(datasets) != 1 || !identity.Identifier(datasets[0]) {
		return DatasetEvidence{}, readexec.ErrBinding
	}
	dataset, err := p.source.DescribeDataset(ctx, e, sources.DatasetDescribeRequest{Source: source.ID, Context: source.ContextID, Dataset: datasets[0]})
	if err != nil {
		return DatasetEvidence{}, err
	}
	if dataset.Source != source.ID || dataset.Context != source.ContextID || dataset.Revision != source.Revision || dataset.Dialect != source.Dialect || dataset.Relation.ID != datasets[0] || !readexec.SQLIdentifier(dataset.Relation.Schema) || !readexec.SQLIdentifier(dataset.Relation.Name) {
		return DatasetEvidence{}, readexec.ErrBinding
	}
	if len(dataset.Relation.Columns) == 0 {
		return DatasetEvidence{}, readexec.ErrBinding
	}
	columns := make([]string, 0, len(dataset.Relation.Columns))
	selectedNames := make([]string, 0, len(dataset.Relation.Columns))
	for _, column := range dataset.Relation.Columns {
		if !column.Safe {
			continue
		}
		if !readexec.SQLIdentifier(column.Name) {
			return DatasetEvidence{}, readexec.ErrBinding
		}
		columns = append(columns, pgx.Identifier{column.Name}.Sanitize())
		selectedNames = append(selectedNames, column.Name)
	}
	if len(columns) == 0 {
		return DatasetEvidence{}, readexec.ErrBinding
	}
	statement := "SELECT " + strings.Join(columns, ",") + " FROM " + pgx.Identifier{dataset.Relation.Schema, dataset.Relation.Name}.Sanitize()
	plan, err := p.validator.Validate(ctx, e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: statement})
	if err != nil {
		return DatasetEvidence{}, err
	}
	rows, err := p.source.Read(ctx, e, plan)
	if err != nil {
		return DatasetEvidence{}, err
	}
	if len(rows.Columns) != len(selectedNames) {
		return DatasetEvidence{}, readexec.ErrBinding
	}
	for i, column := range rows.Columns {
		if column != selectedNames[i] {
			return DatasetEvidence{}, readexec.ErrBinding
		}
	}
	rowHashes := make([]string, len(rows.Values))
	for i, row := range rows.Values {
		if len(row) != len(rows.Columns) {
			return DatasetEvidence{}, readexec.ErrBinding
		}
		raw, err := json.Marshal(row)
		if err != nil {
			return DatasetEvidence{}, readexec.ErrBinding
		}
		sum := sha256.Sum256(raw)
		rowHashes[i] = hex.EncodeToString(sum[:])
	}
	sort.Strings(rowHashes)
	digest := hash(struct {
		Tenant  string
		Source  sources.Source
		Dataset sources.Dataset
		Columns []string
		RowHash []string
	}{e.Tenant(), source, dataset, rows.Columns, rowHashes})
	return DatasetEvidence{SourceID: source.ID, ContextID: source.ContextID, SourceRevision: source.Revision, Digest: digest, Rows: int64(len(rows.Values))}, nil
}
