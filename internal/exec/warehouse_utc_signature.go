package exec

import (
	"strings"

	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
)

// mysqlUTCInstantShape is a deliberately narrow source syntax contract. The
// source's native TIMESTAMP family and authority are checked separately in scope.
func mysqlUTCInstantShape(cast map[string]any) (map[string]any, bool) {
	if !warehouseScopeFields(cast, "this", "to", "double_colon_syntax") || truth(cast["double_colon_syntax"]) {
		return nil, false
	}
	target := object(cast["to"])
	if !warehouseScopeFields(target, "data_type", "name") || text(target["data_type"]) != "custom" || !strings.EqualFold(text(target["name"]), "DATETIME(6)") {
		return nil, false
	}
	zone := object(object(cast["this"])["at_time_zone"])
	if !warehouseScopeFields(zone, "this", "zone") {
		return nil, false
	}
	literal := object(object(zone["zone"])["literal"])
	if !warehouseScopeFields(literal, "literal_type", "value") || text(literal["literal_type"]) != "string" {
		return nil, false
	}
	if value := text(literal["value"]); !sqlpolicy.MySQLUTCZone(value) {
		return nil, false
	}
	column := object(object(zone["this"])["column"])
	if !warehouseScopeFields(column, "name", "table", "join_mark") || truth(column["join_mark"]) {
		return nil, false
	}
	return column, true
}

func (r *warehouseScopeResolver) mysqlUTCInstant(cast map[string]any, s *warehouseReadScope) error {
	column, ok := mysqlUTCInstantShape(cast)
	if r.full.Dialect != "mysql" || !ok {
		return ErrUnsupported
	}
	if err := r.column(column, s); err != nil {
		return err
	}
	name, table := text(object(column["name"])["name"]), text(object(column["table"])["name"])
	for current := s; current != nil; current = current.parent {
		for label, source := range current.sources {
			if table != "" && table != label {
				continue
			}
			if source.columns[name] == 0 {
				continue
			}
			// Logical outputs deliberately carry no physical type proof. Native source
			// width must be known; civil DATETIME and unknown aliases cannot become UTC.
			if sqlpolicy.MySQLTimestampType(source.nativeTypes[name]) {
				return nil
			}
			return ErrUnsupported
		}
	}
	return ErrUnsafe
}
