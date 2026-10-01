package exec

import "strings"

// AnalyticalCalendarKindForDialect maps verified source types, never inferred
// values or session defaults, to the existing calendar contract's temporal roles.
func AnalyticalCalendarKindForDialect(dialect, native, category string) string {
	if dialect == "postgres" {
		return AnalyticalCalendarKind(native, category)
	}
	r := warehouseAnalyticalRelation(dialect, Relation{Columns: []Column{{NativeType: native, Category: category}}})
	return AnalyticalCalendarKind(r.Columns[0].NativeType, category)
}

func (n *warehouseAnalyticalNormalizer) calendarColumn(node any) (Column, bool) {
	c := object(object(node)["column"])
	if !warehouseAnalyticalFields(c, "name", "table") {
		return Column{}, false
	}
	name := text(object(c["name"])["name"])
	qualifier := text(object(c["table"])["name"])
	var out Column
	matches := 0
	for _, r := range n.binding.Relations {
		if len(n.sources) > 0 && !n.sources[r.ID] {
			continue
		}
		if qualifier != "" && n.aliases[qualifier] != r.ID && qualifier != r.Name {
			continue
		}
		for _, col := range r.Columns {
			if col.Name == name && col.Safe {
				out = col
				matches++
			}
		}
	}
	return out, matches == 1
}
func (n *warehouseAnalyticalNormalizer) canonicalCalendar(source any, unit, timezone string, depth int) (any, error) {
	if field, ok := n.mysqlUTCInstant(source); ok {
		if timezone != "" {
			return n.unsupported()
		}
		x, err := n.expr(field, depth+1)
		if err != nil {
			return nil, err
		}
		return map[string]any{"WarehouseCalendarBucket": map[string]any{"source": x, "grain": unit, "timezone": "UTC", "representation": "civil"}}, nil
	}
	col, ok := n.calendarColumn(source)
	if !ok {
		return n.unsupported()
	}
	kind := AnalyticalCalendarKindForDialect(n.binding.Dialect, col.NativeType, col.Category)
	if kind == "" || kind != "instant" && timezone != "" || kind == "instant" && timezone == "" {
		return n.unsupported()
	}
	x, err := n.expr(source, depth+1)
	if err != nil {
		return nil, err
	}
	if kind == "date" {
		x = map[string]any{"TypeCast": map[string]any{"arg": x, "typeName": map[string]any{"TypeName": map[string]any{"names": warehouseNames("timestamp")}}}}
	}
	args := []any{warehouseConst(unit, true), x}
	if kind == "instant" {
		args = append(args, warehouseConst(timezone, true))
	}
	return map[string]any{"FuncCall": map[string]any{"funcname": warehouseNames("date_trunc"), "args": args}}, nil
}

// Native date-part arguments are interpreted only at registered grammar slots.
// The output is a canonical partition expression, not rendered replacement SQL.
func (n *warehouseAnalyticalNormalizer) calendarFunction(name string, args []any, b map[string]any, depth int) (any, bool, error) {
	fail := func() (any, bool, error) { _, err := n.unsupported(); return nil, true, err }
	if !warehouseAnalyticalFields(b, "name", "args", "this") {
		return nil, false, nil
	}
	switch n.binding.Dialect {
	case "bigquery":
		if name != "date_trunc" && name != "datetime_trunc" && name != "timestamp_trunc" {
			return nil, false, nil
		}
		if len(args) < 2 || len(args) > 3 {
			return fail()
		}
		unit, ok := warehouseCalendarUnit(n.binding.Dialect, name, 1, args[1])
		if !ok {
			return fail()
		}
		col, ok := n.calendarColumn(args[0])
		if !ok {
			return fail()
		}
		kind := AnalyticalCalendarKindForDialect(n.binding.Dialect, col.NativeType, col.Category)
		if name == "timestamp_trunc" && kind != "instant" || name == "datetime_trunc" && kind != "civil" {
			return fail()
		}
		timezone := ""
		if kind == "instant" {
			timezone = "UTC"
		}
		if len(args) == 3 {
			if kind != "instant" {
				return fail()
			}
			literal := object(object(args[2])["literal"])
			if text(literal["literal_type"]) != "string" {
				return fail()
			}
			timezone = text(literal["value"])
		}
		x, err := n.canonicalCalendar(args[0], unit, timezone, depth)
		return x, true, err
	case "sqlserver":
		if name != "datetrunc" {
			return nil, false, nil
		}
		if len(args) != 2 {
			return fail()
		}
		unit, ok := warehouseCalendarUnit(n.binding.Dialect, name, 0, args[0])
		if !ok {
			return fail()
		}
		x, err := n.canonicalCalendar(args[1], unit, "", depth)
		return x, true, err
	case "snowflake", "databricks":
		if name != "date_trunc" {
			return nil, false, nil
		}
		if len(args) != 2 {
			return fail()
		}
		unit, ok := warehouseCalendarUnit(n.binding.Dialect, name, 0, args[0])
		if !ok {
			return fail()
		}
		x, err := n.canonicalCalendar(args[1], unit, "", depth)
		return x, true, err
	}
	return nil, false, nil
}

func (n *warehouseAnalyticalNormalizer) mysqlCalendarCast(b map[string]any, depth int) (any, bool, error) {
	if n.binding.Dialect != "mysql" || text(object(b["to"])["data_type"]) != "date" {
		return nil, false, nil
	}
	name, args, body, ok := warehouseCall(object(b["this"]))
	if !ok || name != "date_format" {
		return nil, false, nil
	}
	fail := func() (any, bool, error) { _, err := n.unsupported(); return nil, true, err }
	if len(args) != 2 || !warehouseAnalyticalFields(body, "name", "args") {
		return fail()
	}
	format := object(object(args[1])["literal"])
	if text(format["literal_type"]) != "string" {
		return fail()
	}
	unit := map[string]string{"%Y-%m-%d": "day", "%Y-%m-01": "month", "%Y-01-01": "year"}[text(format["value"])]
	if unit == "" {
		return fail()
	}
	x, err := n.canonicalCalendar(args[0], unit, "", depth)
	if err != nil {
		return nil, true, err
	}
	if object(object(x)["WarehouseCalendarBucket"]) != nil {
		return x, true, nil
	}
	return map[string]any{"TypeCast": map[string]any{"arg": x, "typeName": map[string]any{"TypeName": map[string]any{"names": warehouseNames("date")}}}}, true, nil
}

func warehouseExactNumberValue(node any, want string) bool {
	lit := object(object(node)["literal"])
	if text(lit["literal_type"]) != "number" {
		return false
	}
	value, ok := analyticalNumber(text(lit["value"]))
	return ok && value == want
}
func warehouseUnparen(node any) any {
	for i := 0; i < 32; i++ {
		p := object(object(node)["paren"])
		if p == nil {
			return node
		}
		if !warehouseAnalyticalFields(p, "this") {
			return nil
		}
		node = p["this"]
	}
	return nil
}
func warehouseExtract(node any, unit string) (any, bool) {
	body := object(object(node)["extract"])
	if !warehouseAnalyticalFields(body, "this", "field") || !strings.EqualFold(text(body["field"]), unit) {
		return nil, false
	}
	return body["this"], true
}

// MySQL's complete-year anchor plus zero-based quarter offset preserves years,
// NULLs and calendar boundaries; EXTRACT is native structural evidence.
func (n *warehouseAnalyticalNormalizer) mysqlQuarter(node any, depth int) (any, bool, error) {
	if n.binding.Dialect != "mysql" {
		return nil, false, nil
	}
	add := object(object(node)["add"])
	if add == nil {
		return nil, false, nil
	}
	name, args, body, ok := warehouseCall(object(add["left"]))
	if !ok || name != "makedate" {
		return nil, false, nil
	}
	fail := func() (any, bool, error) { _, err := n.unsupported(); return nil, true, err }
	if !warehouseAnalyticalFields(add, "left", "right") || !warehouseAnalyticalFields(body, "name", "args") || len(args) != 2 || !warehouseExactNumberValue(args[1], "1") {
		return fail()
	}
	source, ok := warehouseExtract(args[0], "year")
	if !ok {
		return fail()
	}
	interval := object(object(add["right"])["interval"])
	unit := object(interval["unit"])
	if !warehouseAnalyticalFields(interval, "this", "unit") || !warehouseAnalyticalFields(unit, "type", "unit", "use_plural") || text(unit["type"]) != "simple" || text(unit["unit"]) != "Quarter" {
		return fail()
	}
	offset := object(object(warehouseUnparen(interval["this"]))["sub"])
	if !warehouseAnalyticalFields(offset, "left", "right") || !warehouseExactNumberValue(offset["right"], "1") {
		return fail()
	}
	same, ok := warehouseExtract(offset["left"], "quarter")
	if !ok {
		return fail()
	}
	if _, ok := n.mysqlUTCInstant(source); ok {
		if Hash(source) != Hash(same) {
			return fail()
		}
		x, err := n.canonicalCalendar(source, "quarter", "", depth)
		return x, true, err
	}
	first, err := n.expr(source, depth+1)
	if err != nil {
		return nil, true, err
	}
	second, err := n.expr(same, depth+1)
	if err != nil {
		return nil, true, err
	}
	if Hash(first) != Hash(second) {
		return fail()
	}
	x, err := n.canonicalCalendar(source, "quarter", "", depth)
	return x, true, err
}
