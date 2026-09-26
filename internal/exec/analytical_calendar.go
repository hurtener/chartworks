package exec

import (
	"strings"
	"time"
)

// AnalyticalCalendarVersion preserves v1/v2 replay while adding reviewed buckets.
const AnalyticalCalendarVersion = "analytical-metrics-v3"
const AnalyticalCalendarPolicy = "reviewed-calendar-suffix-v1"
const AnalyticalCalendarScope = "selected_metric_expression_population_and_calendar_grouping;single_base_relation"

// AnalyticalBucket specifies a partition, not display formatting or query SQL.
// Timezone is mandatory only for instant-valued fields; civil values do not shift.
type AnalyticalBucket struct {
	Column   string `json:"column"`
	Grain    string `json:"grain"`
	Calendar string `json:"calendar"`
	Timezone string `json:"timezone,omitempty"`
}

// AnalyticalCalendarKind recognizes exact PostgreSQL native types, not a broad
// connector category. A temporal category alone cannot establish zone semantics.
func AnalyticalCalendarKind(nativeType, category string) string {
	if category != "temporal" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(nativeType)) {
	case "date":
		return "date"
	case "timestamp", "timestamp without time zone":
		return "civil"
	case "timestamptz", "timestamp with time zone":
		return "instant"
	default:
		return ""
	}
}

func validAnalyticalBucket(b AnalyticalBucket, col Column) bool {
	if !col.Safe || b.Column != col.Name || b.Calendar != "gregorian" {
		return false
	}
	switch b.Grain {
	case "day", "month", "quarter", "year":
	default:
		return false
	}
	switch AnalyticalCalendarKind(col.NativeType, col.Category) {
	case "date", "civil":
		return b.Timezone == ""
	case "instant":
		if b.Timezone == "" || b.Timezone == "Local" || len(b.Timezone) > 128 {
			return false
		}
		_, err := time.LoadLocation(b.Timezone)
		return err == nil
	default:
		return false
	}
}

func analyticalBucketKey(b AnalyticalBucket) string { return "bucket:" + Hash(b) }
func (t analyticalTerm) groupKey() string {
	if t.bucket != "" {
		return t.bucket
	}
	if t.column != "" {
		return "column:" + t.column
	}
	return ""
}

func calendarCast(node any, name string) (any, bool) {
	cast := object(object(node)["TypeCast"])
	if cast == nil {
		return nil, false
	}
	typ := fieldObject(cast["typeName"], "TypeName")
	parts, ok := names(typ["names"])
	if len(parts) == 2 && parts[0] == "pg_catalog" {
		parts = parts[1:]
	}
	if !ok || len(parts) != 1 || parts[0] != name || len(array(typ["typmods"])) > 0 || len(array(typ["arrayBounds"])) > 0 || truth(typ["setof"]) {
		return nil, false
	}
	return cast["arg"], true
}
func calendarLiteral(node any) (string, bool) {
	c := object(object(node)["A_Const"])
	value := object(c["sval"])
	if c == nil || truth(c["isnull"]) || value == nil {
		return "", false
	}
	return text(value["sval"]), true
}

// calendarTerm recognizes only partitions for which field type, unit and zone
// are checked. EXTRACT(month), display strings and session-zone coercions do not
// acquire bucket equivalence. It runs only under the new versioned grain policy.
func (a *analyticalChecker) calendarTerm(node any, depth int) (analyticalTerm, bool, error) {
	bad := func() (analyticalTerm, bool, error) {
		return analyticalTerm{}, true, analyticalFailure("analytical_grain_unsupported", true)
	}
	if arg, ok := calendarCast(node, "date"); ok {
		t, err := a.term(arg, depth+1)
		if err != nil {
			return analyticalTerm{}, true, err
		}
		// Casting a zoned midnight to date consults the session timezone.
		if t.bucket == "" || t.zoned {
			return bad()
		}
		return t, true, nil
	}
	f := object(object(node)["FuncCall"])
	parts, ok := names(f["funcname"])
	if len(parts) == 2 && parts[0] == "pg_catalog" {
		parts = parts[1:]
	}
	if !ok || len(parts) != 1 || parts[0] != "date_trunc" {
		return analyticalTerm{}, false, nil
	}
	if !only(f, "funcname", "args", "location", "funcformat") {
		return bad()
	}
	args := array(f["args"])
	if len(args) != 2 && len(args) != 3 {
		return bad()
	}
	grain, ok := calendarLiteral(args[0])
	if !ok {
		return bad()
	}
	source := args[1]
	casted := false
	if arg, ok := calendarCast(source, "timestamp"); ok {
		source, casted = arg, true
	}
	col, ok := a.field(source)
	if !ok {
		return bad()
	}
	kind := AnalyticalCalendarKind(col.NativeType, col.Category)
	if kind == "date" && !casted || kind == "instant" && casted || kind == "" {
		return bad()
	}
	b := AnalyticalBucket{Column: col.Name, Grain: strings.ToLower(grain), Calendar: "gregorian"}
	if len(args) == 3 {
		if kind != "instant" {
			return bad()
		}
		b.Timezone, ok = calendarLiteral(args[2])
		if !ok {
			return bad()
		}
	}
	if !validAnalyticalBucket(b, col) {
		return bad()
	}
	return analyticalTerm{bucket: analyticalBucketKey(b), zoned: kind == "instant"}, true, nil
}
