package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

type filterDialectAdapter struct{ binding exec.Binding }

func (a *filterDialectAdapter) Binding(context.Context, identity.Envelope, string, string) (exec.Binding, error) {
	return a.binding.Clone(), nil
}

func (a *filterDialectAdapter) Explain(_ context.Context, e identity.Envelope, candidate exec.Candidate) error {
	_, _, err := candidate.SQL(e, a.binding)
	return err
}

func TestFilterCursorTamperExpiryAndTypedLabels(t *testing.T) {
	service := &Documents{cursorKey: [32]byte{1, 2, 3, 4}}
	cursor := filterCursor{Report: "report", Revision: 1, Filter: "region", Search: "nor", Limit: 20, Locale: "en-US", SourceRevision: 3, Authority: strings.Repeat("a", 64), Type: "text", Last: json.RawMessage(`"North"`), Expires: time.Now().Add(time.Minute).Unix()}
	encoded, err := service.encodeFilterCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := service.decodeFilterCursor(encoded)
	if err != nil || decoded.Report != cursor.Report || string(decoded.Last) != string(cursor.Last) {
		t.Fatal("cursor round trip", decoded, err)
	}
	tampered := encoded[:len(encoded)-1] + "x"
	if strings.HasSuffix(encoded, "x") {
		tampered = encoded[:len(encoded)-1] + "y"
	}
	if _, err := service.decodeFilterCursor(tampered); err == nil {
		t.Fatal("tampered cursor accepted")
	}
	cursor.Expires = time.Now().Add(-time.Second).Unix()
	expired, err := service.encodeFilterCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.decodeFilterCursor(expired); !errors.Is(err, ErrStale) {
		t.Fatal("expired cursor classification", err)
	}
	for _, test := range []struct {
		raw, locale, label string
	}{{`null`, "es-AR", "Nulo"}, {`true`, "en-US", "true"}, {`"9007199254740993.125"`, "en-US", "9007199254740993.125"}} {
		label, err := filterLabel(json.RawMessage(test.raw), test.locale)
		if err != nil || label != test.label {
			t.Fatal(test.raw, label, err)
		}
	}
}

func TestFilterOptionSourceValidation(t *testing.T) {
	valid := Parameter{Name: "region", Type: "dimension_value", Required: true, Dimension: &DimensionReference{Topic: "topic", Version: "v1", Dimension: "region"}}
	source := &FilterOptionSource{Version: 1, Block: "block", BlockRevision: 1, Topic: "topic", TopicVersion: "v1", Dataset: "sales", Column: "region"}
	if !validFilterOptionSource(valid, source) {
		t.Fatal("valid exact source rejected")
	}
	bad := *source
	bad.BlockRevision = 0
	if validFilterOptionSource(valid, &bad) || validFilterOptionSource(Parameter{Name: "period", Type: "relative_period"}, source) {
		t.Fatal("floating or non-scalar option source accepted")
	}
}

func TestFilterOptionDialectMatrixUsesNativeAdmission(t *testing.T) {
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.query", "cw.source.query:source", "cw.execution_context.use:source:v1", "cw.dataset.query:sales"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		dialect string
		want    []string
	}{
		{"postgres", []string{`"analytics"."sales"`, "$1", "NULLS LAST", "LIMIT 11"}},
		{"mysql", []string{"`analytics`.`sales`", "?", "DESC", "LIMIT 11"}},
		{"sqlserver", []string{"[warehouse].[analytics].[sales]", "@p1", "TOP (11)", "DESC"}},
		{"bigquery", []string{"`warehouse.analytics.sales`", "@p1", "NULLS LAST", "LIMIT 11"}},
		{"snowflake", []string{`"warehouse"."analytics"."sales"`, "?", "NULLS LAST", "LIMIT 11"}},
		{"databricks", []string{"`warehouse`.`analytics`.`sales`", "?", "NULLS LAST", "LIMIT 11"}},
	} {
		dialect := test.dialect
		t.Run(dialect, func(t *testing.T) {
			catalog := ""
			if dialect != "postgres" && dialect != "mysql" {
				catalog = "warehouse"
			}
			binding := exec.Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: dialect, Catalog: catalog, Contract: "contract", Fingerprint: exec.Hash("filter-options"), Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "name", NativeType: "text", Category: "text", Nullable: true, Safe: true}}}}}
			statement, err := filterOptionStatement(binding, binding.Relations[0], binding.Relations[0].Columns[0], true, true, 11)
			if err != nil || !strings.Contains(statement, "ESCAPE '!'") || strings.Contains(statement, "NULLS LAST") == (dialect == "mysql" || dialect == "sqlserver") {
				t.Fatal("closed dialect builder", statement, err)
			}
			for _, wanted := range test.want {
				if !strings.Contains(statement, wanted) {
					t.Fatal("dialect clause absent", wanted, statement)
				}
			}
			adapter := &filterDialectAdapter{binding: binding}
			validator, err := exec.NewValidator(adapter, config.DefaultReadValidation())
			if err != nil {
				t.Fatal(err)
			}
			plan, err := validator.ValidateWithin(t.Context(), e, exec.Request{Source: "source", Context: "source:v1", SQL: statement, Parameters: []exec.Parameter{{Kind: "text", Value: "%x!%%"}, {Kind: "text", Value: "x"}}}, []exec.RelationScope{{Dataset: "sales", Columns: []string{"name"}}})
			if err != nil || !plan.Receipt().Validated || plan.Receipt().Dialect != dialect {
				t.Fatal("native dialect admission", statement, plan.Receipt(), err)
			}
		})
	}
}

func TestFilterCursorScalarBoundaries(t *testing.T) {
	for _, size := range []int{128, 129, 1024} {
		raw, _ := json.Marshal(strings.Repeat("x", size))
		if _, err := filterParameter("text", raw); err != nil {
			t.Fatal("cursor-safe scalar rejected", size, err)
		}
	}
	raw, _ := json.Marshal(strings.Repeat("x", 1025))
	if _, err := filterParameter("text", raw); !errors.Is(err, ErrBudget) {
		t.Fatal("oversized cursor scalar accepted", err)
	}
}

func TestFilterOptionTruncationNeverClaimsCompleteness(t *testing.T) {
	for _, truncation := range []string{"bytes", "rows"} {
		result := &exec.Result{Schema: []exec.Field{{Name: "value", Type: "text"}}, Rows: [][]json.RawMessage{{json.RawMessage(`"retained"`)}}, Outcome: "truncated", Truncation: truncation, Bytes: 512 << 10}
		if err := validateFilterOptionResult(result, 10); !errors.Is(err, ErrBudget) {
			t.Fatal("truncated prefix accepted as complete", truncation, err)
		}
	}
}

func TestFilterOptionSemanticPhysicalEqualityFence(t *testing.T) {
	binding := exec.Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Dialect: "postgres", Contract: "contract", Fingerprint: exec.Hash("option-fence"), Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "amount", NativeType: "numeric", Category: "decimal", Nullable: true, Safe: true}}}}}
	publication := topics.Published{Definition: topics.Definition{Topic: "topic", Version: "v1", Datasets: []topics.Dataset{{ID: "sales", Source: topics.Binding{Source: "source", Context: "source:v1", SourceRevision: 1, Dataset: "sales"}, Columns: []semantics.Column{{ID: "amount", SourceName: "amount", NativeType: "numeric", Category: "decimal", Nullable: true}}}}}}
	if _, err := validationScope(binding, []topics.Published{publication}); err != nil {
		t.Fatal("valid semantic/physical equality rejected", err)
	}
	for _, mutate := range []func(*exec.Column){
		func(c *exec.Column) { c.NativeType = "text" },
		func(c *exec.Column) { c.Category = "text" },
		func(c *exec.Column) { c.Nullable = false },
		func(c *exec.Column) { c.Safe = false },
	} {
		drifted := binding.Clone()
		mutate(&drifted.Relations[0].Columns[0])
		if _, err := validationScope(drifted, []topics.Published{publication}); !errors.Is(err, ErrStale) {
			t.Fatal("semantic/source drift accepted", drifted.Relations[0].Columns[0], err)
		}
	}
}
