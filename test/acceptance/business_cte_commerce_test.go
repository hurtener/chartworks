package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

// This case uses the disposable PostgreSQL 17 commerce source and the ordinary
// native validator after binding. It tests value and row grain, not only SQL text.
func TestCommerceBoundCTENetRevenue(t *testing.T) {
	f := liveCommerceSource(t)
	source := f.create(t, "commerce-cte-binding")
	binding, err := f.s.Binding(t.Context(), f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, relation := range binding.Relations {
		ids[relation.Name] = relation.ID
	}
	constraint := readexec.BusinessConstraint{
		Resolution: readexec.Hash("reviewed-Jan-Mar-2026"), Dataset: ids["orders"], Column: "ordered_at", SourceRevision: binding.Revision,
		Kind: "time_window", Operator: "range", Nulls: "exclude", Bounds: "[)", TemporalType: "date", Calendar: "gregorian", TimeZone: "UTC", Grain: "month", Value: "2026-01-01", Upper: "2026-04-01",
	}
	sql := `WITH paid AS (SELECT o.order_id, o.total_usd FROM analytics.orders o WHERE o.status='paid'), refunds_by_order AS (SELECT r.order_id, SUM(r.amount_usd) AS refunded FROM analytics.refunds r GROUP BY r.order_id) SELECT SUM(p.total_usd - COALESCE(r.refunded,0)) AS net_revenue FROM paid p LEFT JOIN refunds_by_order r ON r.order_id=p.order_id`
	bound, err := readexec.BindBusinessConstraints(t.Context(), binding, sql, nil, []readexec.BusinessConstraint{constraint})
	if err != nil || len(bound.Parameters) != 2 || bound.Parameters[0].Value != "2026-01-01" || bound.Parameters[1].Value != "2026-04-01" || !strings.Contains(bound.SQL, `"o"."ordered_at" >=`) || !strings.Contains(bound.SQL, `"o"."ordered_at" <`) {
		t.Fatalf("lost reviewed half-open base-row interval: %v", err)
	}
	scope := []readexec.RelationScope{
		{Dataset: ids["orders"], Columns: []string{"order_id", "total_usd", "status", "ordered_at"}},
		{Dataset: ids["refunds"], Columns: []string{"order_id", "amount_usd"}},
	}
	plan, err := f.validator.ValidateWithin(t.Context(), f.e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: bound.SQL, Parameters: bound.Parameters}, scope)
	if err != nil {
		t.Fatal("native validated read rejected bound grain-safe query", err)
	}
	rows, err := f.s.Read(t.Context(), f.e, plan)
	if err != nil || len(rows.Values) != 1 || len(rows.Values[0]) != 1 || rows.Values[0][0] == nil || *rows.Values[0][0] != "515.00" {
		t.Fatalf("net revenue lost order/refund grain: %v %#v", err, rows.Values)
	}
	for _, sql := range []string{
		`WITH paid AS (SELECT o.order_id,o.total_usd FROM analytics.orders o), refunds AS (SELECT r.order_id,r.amount_usd FROM analytics.refunds r JOIN analytics.orders o ON o.order_id=r.order_id) SELECT * FROM paid p JOIN refunds r ON p.order_id=r.order_id`,
		`WITH paid AS (SELECT o.order_id FROM analytics.orders o) SELECT 1`,
	} {
		out, err := readexec.BindBusinessConstraints(context.Background(), binding, sql, nil, []readexec.BusinessConstraint{constraint})
		if !errors.Is(err, readexec.ErrUnsupported) || out.SQL != "" {
			t.Fatal("ambiguous or unused target produced executable SQL", err)
		}
	}
}

func TestCommerceCTEOuterJoinTargetFailsClosed(t *testing.T) {
	f := liveCommerceSource(t)
	source := f.create(t, "commerce-outer-join-binding")
	binding, err := f.s.Binding(t.Context(), f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	var ordersID string
	for _, relation := range binding.Relations {
		if relation.Name == "orders" {
			ordersID = relation.ID
		}
	}
	if ordersID == "" {
		t.Fatal("missing reviewed orders relation")
	}
	constraint := readexec.BusinessConstraint{
		Resolution: readexec.Hash("reviewed-order-minimum"), Dataset: ordersID, Column: "total_usd", SourceRevision: binding.Revision,
		Kind: "number", Operator: "gte", Nulls: "exclude", Unit: "USD", Precision: 12, Scale: 2, Value: "150.00",
	}
	// Correct prejoin filtering retains customers 1 and 3 with NULL orders;
	// applying the same predicate in WHERE after LEFT JOIN loses those rows.
	var correctRows, wrongRows int
	if err := f.admin.QueryRow(t.Context(), `SELECT count(*) FROM analytics.customers c LEFT JOIN (SELECT * FROM analytics.orders WHERE total_usd >= 150) o ON o.customer_id=c.customer_id`).Scan(&correctRows); err != nil {
		t.Fatal(err)
	}
	if err := f.admin.QueryRow(t.Context(), `SELECT count(*) FROM analytics.customers c LEFT JOIN analytics.orders o ON o.customer_id=c.customer_id WHERE o.total_usd >= 150`).Scan(&wrongRows); err != nil {
		t.Fatal(err)
	}
	if correctRows != 4 || wrongRows != 2 {
		t.Fatalf("fixture no longer proves outer-join row loss: prejoin=%d where=%d", correctRows, wrongRows)
	}
	sql := `WITH joined AS (SELECT c.customer_id,o.total_usd FROM analytics.customers c LEFT JOIN analytics.orders o ON o.customer_id=c.customer_id) SELECT customer_id,total_usd FROM joined`
	out, err := readexec.BindBusinessConstraints(t.Context(), binding, sql, nil, []readexec.BusinessConstraint{constraint})
	var detail *readexec.BusinessConstraintError
	if !errors.As(err, &detail) || detail.Code != "unsupported_outer_join_target" || out.SQL != "" || len(out.Parameters) != 0 {
		t.Fatal("nullable-side target did not fail closed", err)
	}
}
