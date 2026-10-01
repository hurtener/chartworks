package acceptance

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const adversarialCorpusDirectory = "testdata/generated_adversarial"

type adversarialOrder struct {
	Division string  `json:"division_id"`
	ID       int     `json:"order_id"`
	Customer int     `json:"customer_id"`
	At       string  `json:"ordered_at"`
	Amount   *string `json:"misleading_net_total_usd"`
	Status   *string `json:"status_code"`
}
type adversarialRefund struct {
	Division string  `json:"division_id"`
	ID       int     `json:"refund_id"`
	Order    int     `json:"order_id"`
	At       string  `json:"refunded_at"`
	Amount   *string `json:"amount_usd"`
	Status   *string `json:"status_code"`
}
type adversarialCustomer struct {
	Division string  `json:"division_id"`
	ID       int     `json:"customer_id"`
	Segment  *string `json:"segment"`
}
type adversarialLine struct {
	Division string `json:"division_id"`
	ID       int    `json:"line_id"`
	Order    int    `json:"order_id"`
	Category string `json:"category"`
	Amount   string `json:"line_total_usd"`
}
type adversarialWarehouse struct {
	SchemaVersion int                   `json:"schema_version"`
	Timezone      string                `json:"timezone"`
	Orders        []adversarialOrder    `json:"orders"`
	Refunds       []adversarialRefund   `json:"refunds"`
	Customers     []adversarialCustomer `json:"customers"`
	Lines         []adversarialLine     `json:"order_lines"`
	PrivateNote   string                `json:"private_note_canary"`
}
type adversarialAuthorInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Values      []struct {
		Table   string   `json:"table"`
		Column  string   `json:"column"`
		Label   string   `json:"label"`
		Value   string   `json:"stored_value"`
		Aliases []string `json:"aliases"`
	} `json:"vocabulary_labels"`
	Privacy map[string]string `json:"privacy_annotations"`
}
type adversarialCase struct {
	ID       string  `json:"id"`
	Question string  `json:"question"`
	Expected string  `json:"expected_outcome"`
	Oracle   *string `json:"oracle_key"`
}
type adversarialCaseResult struct {
	AmountCompleteness any     `json:"amount_completeness,omitempty"`
	Oracle             *string `json:"oracle_key,omitempty"`
	Result             any     `json:"result,omitempty"`
	ID                 string  `json:"id"`
	Expected           string  `json:"expected"`
	Actual             string  `json:"actual"`
	Matches            bool    `json:"matches"`
	Stage              string  `json:"stage"`
	Reason             string  `json:"reason,omitempty"`
}

func readAdversarialJSON(t *testing.T, name string, target any) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(adversarialCorpusDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, target); err != nil {
		t.Fatal(name, err)
	}
	return raw
}

// This independent Go oracle never reads model output, SQL, topic definitions,
// model prompts or held-out questions. Money is exact rational arithmetic.
func adversarialOracle(t *testing.T, warehouse adversarialWarehouse) map[string]any {
	t.Helper()
	zone, err := time.LoadLocation(warehouse.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	local := func(value string) time.Time {
		v, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return v.In(zone)
	}
	money := func(value *string) *big.Rat {
		if value == nil {
			return nil
		}
		v, ok := new(big.Rat).SetString(*value)
		if !ok {
			t.Fatal("invalid exact fixture money")
		}
		return v
	}
	type orderKey struct {
		division string
		id       int
	}
	cohort, allPaid := map[orderKey]bool{}, map[orderKey]bool{}
	gross := new(big.Rat)
	months := map[string]*big.Rat{}
	monthUnknown := map[string]int{}
	paidCount, knownCount, unknownCount := 0, 0, 0
	for _, order := range warehouse.Orders {
		if order.Status == nil || *order.Status != "P" {
			continue
		}
		key := orderKey{order.Division, order.ID}
		allPaid[key] = true
		at := local(order.At)
		if at.Year() != 2026 {
			continue
		}
		cohort[key] = true
		paidCount++
		month := at.Format("2006-01")
		if _, ok := monthUnknown[month]; !ok {
			monthUnknown[month] = 0
		}
		if _, ok := months[month]; !ok {
			months[month] = nil
		}
		if amount := money(order.Amount); amount != nil {
			knownCount++
			gross.Add(gross, amount)
			if months[month] == nil {
				months[month] = new(big.Rat)
			}
			months[month].Add(months[month], amount)
		} else {
			unknownCount++
			monthUnknown[month]++
		}
	}
	cohortRefund, activityRefund := new(big.Rat), new(big.Rat)
	unknownRefund, unknownActivityRefund := 0, 0
	for _, refund := range warehouse.Refunds {
		if refund.Status == nil || *refund.Status != "P" {
			continue
		}
		key := orderKey{refund.Division, refund.Order}
		amount := money(refund.Amount)
		if cohort[key] {
			if amount == nil {
				unknownRefund++
			} else {
				cohortRefund.Add(cohortRefund, amount)
			}
		}
		if allPaid[key] && local(refund.At).Year() == 2026 {
			if amount != nil {
				activityRefund.Add(activityRefund, amount)
			} else {
				unknownActivityRefund++
			}
		}
	}
	monthValues := map[string]any{}
	for key, value := range months {
		if value == nil {
			monthValues[key] = nil
		} else {
			monthValues[key] = value.FloatString(2)
		}
	}
	return map[string]any{"known_gross_paid_local_2026": gross.FloatString(2), "definitive_gross_paid_local_2026": nil, "paid_order_count_local_2026": paidCount, "known_paid_amount_count_local_2026": knownCount, "monthly_qualifying_paid_local_2026": monthValues, "monthly_unknown_paid_amounts_local_2026": monthUnknown, "posted_known_refunds_for_2026_paid_order_cohort_all_event_dates": cohortRefund.FloatString(2), "posted_known_refunds_on_paid_orders_by_2026_refund_activity": activityRefund.FloatString(2), "known_cohort_net": new(big.Rat).Sub(gross, cohortRefund).FloatString(2), "known_activity_net": new(big.Rat).Sub(gross, activityRefund).FloatString(2), "unknown_posted_refund_events_on_paid_orders_by_2026_activity": unknownActivityRefund, "unknown_amount_paid_orders": unknownCount, "unknown_posted_refund_events_in_2026_paid_cohort": unknownRefund}
}

func TestGeneratedAdversarialCorpusOracleAndIsolation(t *testing.T) {
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	var expected map[string]any
	readAdversarialJSON(t, "oracle.json", &expected)
	actual := adversarialOracle(t, warehouse)
	for key, value := range actual {
		a, _ := json.Marshal(value)
		b, _ := json.Marshal(expected[key])
		if string(a) != string(b) {
			t.Fatalf("independent oracle mismatch %s: Go=%s recorded=%s", key, a, b)
		}
	}
	var inputs adversarialAuthorInput
	authorBytes := readAdversarialJSON(t, "authoring_inputs.json", &inputs)
	var netInput map[string]any
	authorBytes = append(authorBytes, readAdversarialJSON(t, "net_business.json", &netInput)...)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	if len(cases) != 19 || len(warehouse.Orders) != 17 || len(warehouse.Refunds) != 13 {
		t.Fatal("finite corpus inventory changed without explicit review")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || seen[c.ID] || c.Question == "" || c.Expected != "answer" && c.Expected != "clarify" && c.Expected != "reject" {
			t.Fatal("invalid held-out case", c.ID)
		}
		seen[c.ID] = true
		if strings.Contains(string(authorBytes), c.Question) {
			t.Fatal("held-out question leaked into authoring input", c.ID)
		}
		if c.Oracle != nil {
			if _, ok := expected[*c.Oracle]; !ok {
				t.Fatal("missing independent answer", c.ID)
			}
		}
	}
	for _, forbidden := range []string{warehouse.PrivateNote, "995.00", "813.00", "805.00", "expected_outcome", "oracle_key", "held_out"} {
		if strings.Contains(string(authorBytes), forbidden) {
			t.Fatal("oracle or row canary leaked into authoring inputs")
		}
	}
	if inputs.Privacy["orders.private_note"] != "sensitive" {
		t.Fatal("private note lost explicit classification")
	}
	if !reflect.DeepEqual(actual["monthly_qualifying_paid_local_2026"].(map[string]any)["2026-06"], nil) || actual["monthly_qualifying_paid_local_2026"].(map[string]any)["2026-10"] != "0.00" {
		t.Fatal("unknown and zero conflated")
	}
}

func seedGeneratedAdversarialWarehouse(t *testing.T, f *engineeringFixture) adversarialWarehouse {
	t.Helper()
	var data adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &data)
	schema, err := os.ReadFile(filepath.Join(adversarialCorpusDirectory, "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.admin.Exec(t.Context(), string(schema)); err != nil {
		t.Fatal(err)
	}
	number := func(value *string) any {
		if value == nil {
			return nil
		}
		var n pgtype.Numeric
		if err := n.Scan(*value); err != nil {
			t.Fatal(err)
		}
		return n
	}
	instant := func(value string) time.Time {
		out, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, v := range data.Customers {
		if _, err = f.admin.Exec(t.Context(), `INSERT INTO analytics.adv_customers VALUES($1,$2,$3)`, v.Division, v.ID, v.Segment); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range data.Orders {
		if _, err = f.admin.Exec(t.Context(), `INSERT INTO analytics.adv_orders VALUES($1,$2,$3,$4,$5,$6,$7)`, v.Division, v.ID, v.Customer, instant(v.At), number(v.Amount), v.Status, data.PrivateNote); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range data.Refunds {
		if _, err = f.admin.Exec(t.Context(), `INSERT INTO analytics.adv_refunds VALUES($1,$2,$3,$4,$5,$6)`, v.Division, v.ID, v.Order, instant(v.At), number(v.Amount), v.Status); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range data.Lines {
		if _, err = f.admin.Exec(t.Context(), `INSERT INTO analytics.adv_order_lines VALUES($1,$2,$3,$4,$5)`, v.Division, v.ID, v.Order, v.Category, number(&v.Amount)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.admin.Exec(t.Context(), "GRANT SELECT ON ALL TABLES IN SCHEMA analytics TO "+pgx.Identifier{f.role}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGeneratedAdversarialWarehouseHasRealTraps(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	data := seedGeneratedAdversarialWarehouse(t, f)
	oracle := adversarialOracle(t, data)
	var gross, wrongCustomer, cohortRefund, activityRefund string
	err := f.admin.QueryRow(t.Context(), `WITH paid AS(SELECT * FROM analytics.adv_orders WHERE status_code='P' AND ordered_at >= TIMESTAMPTZ '2026-01-01 00:00 America/New_York' AND ordered_at < TIMESTAMPTZ '2027-01-01 00:00 America/New_York') SELECT (SELECT sum(misleading_net_total_usd)::text FROM paid),(SELECT sum(o.misleading_net_total_usd)::text FROM paid o JOIN analytics.adv_customers c ON c.customer_id=o.customer_id),(SELECT sum(r.amount_usd)::text FROM analytics.adv_refunds r JOIN paid o ON(o.division_id,o.order_id)=(r.division_id,r.order_id) WHERE r.status_code='P'),(SELECT sum(r.amount_usd)::text FROM analytics.adv_refunds r JOIN analytics.adv_orders o ON(o.division_id,o.order_id)=(r.division_id,r.order_id) WHERE r.status_code='P' AND o.status_code='P' AND r.refunded_at>=TIMESTAMPTZ '2026-01-01 00:00 America/New_York' AND r.refunded_at<TIMESTAMPTZ '2027-01-01 00:00 America/New_York')`).Scan(&gross, &wrongCustomer, &cohortRefund, &activityRefund)
	if err != nil || !liveNumberEquals(gross, oracle["known_gross_paid_local_2026"].(string)) || liveNumberEquals(gross, wrongCustomer) || !liveNumberEquals(cohortRefund, oracle["posted_known_refunds_for_2026_paid_order_cohort_all_event_dates"].(string)) || !liveNumberEquals(activityRefund, oracle["posted_known_refunds_on_paid_orders_by_2026_refund_activity"].(string)) {
		t.Fatal("real warehouse did not preserve independent traps", err)
	}
	var fanout string
	if err := f.admin.QueryRow(t.Context(), `SELECT sum(o.misleading_net_total_usd)::text FROM analytics.adv_orders o LEFT JOIN analytics.adv_order_lines l ON (l.division_id,l.order_id)=(o.division_id,o.order_id) LEFT JOIN analytics.adv_refunds r ON (r.division_id,r.order_id)=(o.division_id,o.order_id) WHERE o.status_code='P' AND o.ordered_at>=TIMESTAMPTZ '2026-01-01 00:00 America/New_York' AND o.ordered_at<TIMESTAMPTZ '2027-01-01 00:00 America/New_York'`).Scan(&fanout); err != nil || liveNumberEquals(fanout, gross) {
		t.Fatal("raw child fanout trap missing", err)
	}
	var foldWallTimes, foldInstants int
	if err := f.admin.QueryRow(t.Context(), `SELECT count(DISTINCT ordered_at AT TIME ZONE 'America/New_York'),count(DISTINCT ordered_at) FROM analytics.adv_orders WHERE (ordered_at AT TIME ZONE 'America/New_York')=TIMESTAMP '2026-11-01 01:30:00'`).Scan(&foldWallTimes, &foldInstants); err != nil || foldWallTimes != 1 || foldInstants != 2 {
		t.Fatal("DST fold trap missing", err, foldWallTimes, foldInstants)
	}
	rows, err := f.admin.Query(t.Context(), `SELECT to_char(ordered_at AT TIME ZONE 'America/New_York','YYYY-MM'),sum(misleading_net_total_usd)::text FROM analytics.adv_orders WHERE status_code='P' AND ordered_at>=TIMESTAMPTZ '2026-01-01 00:00 America/New_York' AND ordered_at<TIMESTAMPTZ '2027-01-01 00:00 America/New_York' GROUP BY 1 ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]any{}
	for rows.Next() {
		var month string
		var amount *string
		if err := rows.Scan(&month, &amount); err != nil {
			t.Fatal(err)
		}
		if amount == nil {
			got[month] = nil
		} else {
			got[month] = *amount
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := oracle["monthly_qualifying_paid_local_2026"].(map[string]any)
	keys := []string{}
	for key := range got {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("monthly warehouse differs from independent oracle: keys=%v", keys)
	}
}
