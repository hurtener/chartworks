package exec

import (
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"strings"
)

// mysqlUTCInstant recognizes an explicit native retrieval conversion, independent
// of the connection's session zone. Precision six preserves every MySQL stored
// fractional second; a truncating temporal cast cannot receive this proof.
func (n *warehouseAnalyticalNormalizer) mysqlUTCInstant(node any) (any, bool) {
	if n.binding.Dialect != "mysql" {
		return nil, false
	}
	cast := object(object(node)["cast"])
	if !warehouseAnalyticalFields(cast, "this", "to", "double_colon_syntax") || truth(cast["double_colon_syntax"]) {
		return nil, false
	}
	typ := object(cast["to"])
	if !warehouseAnalyticalFields(typ, "data_type", "name") || text(typ["data_type"]) != "custom" || !strings.EqualFold(text(typ["name"]), "DATETIME(6)") {
		return nil, false
	}
	zone := object(object(cast["this"])["at_time_zone"])
	if !warehouseAnalyticalFields(zone, "this", "zone") {
		return nil, false
	}
	literal := object(object(zone["zone"])["literal"])
	if !warehouseAnalyticalFields(literal, "literal_type", "value") || text(literal["literal_type"]) != "string" || !sqlpolicy.MySQLUTCZone(text(literal["value"])) {
		return nil, false
	}
	col, ok := n.calendarColumn(zone["this"])
	if !ok || !sqlpolicy.MySQLTimestampType(col.NativeType) || col.Category != "temporal" {
		return nil, false
	}
	return zone["this"], true
}

// This node is produced only by the warehouse structural adapter after matching
// the full native UTC conversion and bucket syntax. It cannot arise from a
// PostgreSQL parse, and carries partition identity separately from output type.
func (a *analyticalChecker) warehouseCalendarTerm(node any) (analyticalTerm, bool, error) {
	b := object(object(node)["WarehouseCalendarBucket"])
	if b == nil {
		return analyticalTerm{}, false, nil
	}
	fail := func() (analyticalTerm, bool, error) {
		return analyticalTerm{}, true, analyticalFailure("analytical_grain_unsupported", true)
	}
	if a.binding.Dialect != "mysql" || !only(b, "source", "grain", "timezone", "representation") || text(b["timezone"]) != "UTC" || text(b["representation"]) != "civil" {
		return fail()
	}
	col, ok := a.field(b["source"])
	if !ok || AnalyticalCalendarKind(col.NativeType, col.Category) != "instant" {
		return fail()
	}
	bucket := AnalyticalBucket{Column: col.Name, Grain: text(b["grain"]), Calendar: "gregorian", Timezone: "UTC"}
	if !validAnalyticalBucket(bucket, col) {
		return fail()
	}
	return analyticalTerm{bucket: analyticalBucketKey(bucket), zoned: false}, true, nil
}
