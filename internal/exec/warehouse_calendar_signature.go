package exec

import (
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"strings"
)

// warehouseCalendarUnit recognizes only a registered date-part grammar slot.
// A data-column occurrence with the same name elsewhere remains a dependency.
func warehouseCalendarUnit(dialect, name string, index int, arg any) (string, bool) {
	name = strings.ToLower(name)
	form, ok := sqlpolicy.CalendarArgument(dialect, name, index)
	if !ok {
		return "", false
	}
	if form == "string" {
		literal := object(object(arg)["literal"])
		if text(literal["literal_type"]) != "string" {
			return "", false
		}
		return sqlpolicy.CalendarUnit(dialect, name, index, form, text(literal["value"]))
	}
	if name == "extract" {
		// This marker is constructed only from the native Extract.field enum.
		value := text(object(arg)["calendar_unit"])
		return sqlpolicy.CalendarUnit(dialect, name, index, form, value)
	}
	column := object(object(arg)["column"])
	identifier := object(column["name"])
	if column == nil || identifier == nil || column["table"] != nil || truth(identifier["quoted"]) || truth(column["join_mark"]) {
		return "", false
	}
	return sqlpolicy.CalendarUnit(dialect, name, index, form, text(identifier["name"]))
}

type warehouseCallEvidence struct {
	tree map[string]any
}
