package reporting

import "slices"

func compatibleFilter(filter, parameter Parameter) bool {
	return filter.Type == parameter.Type && digest(filter.Dimension) == digest(parameter.Dimension)
}

func validateArgument(p Parameter, value Value) error {
	copy := p
	copy.Default = &value
	return validateDeclarations([]Parameter{copy}, 1)
}

// ValidateWidgetBindings checks names, types and all author-supplied values.
// Runtime values are checked again against both filter and block constraints.
func ValidateWidgetBindings(parameters []Parameter, d DocumentDefinition, w Widget) error {
	declared := map[string]Parameter{}
	for _, p := range parameters {
		declared[p.Name] = p
	}
	filters := map[string]Parameter{}
	for _, f := range d.Filters {
		filters[f.Parameter.Name] = f.Parameter
	}
	for _, a := range d.Defaults {
		if p, ok := declared[a.Name]; ok && validateArgument(p, a.Value) != nil {
			return ErrInvalid
		}
	}
	for _, a := range w.Literals {
		if p, ok := declared[a.Name]; !ok || validateArgument(p, a.Value) != nil {
			return ErrInvalid
		}
	}
	for _, b := range w.Bindings {
		filter, found := filters[b.Filter]
		parameter, exists := declared[b.Parameter]
		if !found || !exists || !compatibleFilter(filter, parameter) || filter.Default != nil && validateArgument(parameter, *filter.Default) != nil {
			return ErrInvalid
		}
	}
	for _, name := range w.Overrides {
		if _, ok := declared[name]; !ok {
			return ErrInvalid
		}
	}
	return nil
}

// ResolveWidgetParameters applies RFC-002 precedence without constructing SQL.
// Provenance remains per widget even when equal values share a query.
func ResolveWidgetParameters(parameters []Parameter, d DocumentDefinition, w Widget, filters, overrides []Argument, resolution Resolution) (Resolved, error) {
	_, resolved, err := widgetArguments(parameters, d, w, filters, overrides, resolution)
	return resolved, err
}

// widgetArguments retains the typed values as well as their resolved execution
// parameters, so a child run can replay a relative period at the accepted time.
func widgetArguments(parameters []Parameter, d DocumentDefinition, w Widget, filters, overrides []Argument, resolution Resolution) ([]Argument, Resolved, error) {
	if w.Block == nil || ValidateWidgetBindings(parameters, d, w) != nil || len(filters) > len(d.Filters) || len(overrides) > len(w.Overrides) {
		return nil, Resolved{}, ErrInvalid
	}
	declared := map[string]Parameter{}
	for _, p := range parameters {
		declared[p.Name] = p
	}
	values, provenance := map[string]Value{}, map[string]string{}
	apply := func(layer []Argument, origin string, ignoreUnknown bool) error {
		seen := map[string]bool{}
		for _, a := range layer {
			if seen[a.Name] {
				return ErrInvalid
			}
			seen[a.Name] = true
			p, ok := declared[a.Name]
			if !ok {
				if ignoreUnknown {
					continue
				}
				return ErrInvalid
			}
			if validateArgument(p, a.Value) != nil {
				return ErrInvalid
			}
			values[a.Name], provenance[a.Name] = clone(a.Value), origin
		}
		return nil
	}
	if apply(d.Defaults, "report_default", true) != nil || apply(w.Literals, "widget_literal", false) != nil {
		return nil, Resolved{}, ErrInvalid
	}
	filterValues, err := resolveReportFilters(d.Filters, filters)
	if err != nil {
		return nil, Resolved{}, err
	}
	bound := map[string]bool{}
	for _, b := range w.Bindings {
		if bound[b.Parameter] {
			return nil, Resolved{}, ErrInvalid
		}
		bound[b.Parameter] = true
		if value, exists := filterValues[b.Filter]; exists {
			if validateArgument(declared[b.Parameter], value) != nil {
				return nil, Resolved{}, ErrInvalid
			}
			values[b.Parameter], provenance[b.Parameter] = clone(value), "filter:"+b.Filter
		}
	}
	for _, a := range overrides {
		if !slices.Contains(w.Overrides, a.Name) {
			return nil, Resolved{}, ErrInvalid
		}
	}
	if apply(overrides, "invocation_override", false) != nil {
		return nil, Resolved{}, ErrInvalid
	}
	arguments := []Argument{}
	for _, p := range parameters {
		if value, ok := values[p.Name]; ok {
			arguments = append(arguments, Argument{Name: p.Name, Value: value})
		}
	}
	resolved, err := ResolveParameters(parameters, arguments, resolution)
	if err != nil {
		return nil, Resolved{}, err
	}
	for i := range resolved.Values {
		p := provenance[resolved.Values[i].Name]
		if p == "" {
			p = "block_default"
		}
		resolved.Values[i].Provenance = p
	}
	return arguments, resolved, nil
}

func resolveReportFilters(definitions []ReportFilter, arguments []Argument) (map[string]Value, error) {
	definitionsByName := map[string]Parameter{}
	values := map[string]Value{}
	for _, f := range definitions {
		p := f.Parameter
		if definitionsByName[p.Name].Name != "" || validateDeclarations([]Parameter{p}, 1) != nil {
			return nil, ErrInvalid
		}
		definitionsByName[p.Name] = p
		if p.Default != nil {
			values[p.Name] = clone(*p.Default)
		}
	}
	seen := map[string]bool{}
	for _, a := range arguments {
		p, ok := definitionsByName[a.Name]
		if !ok || seen[a.Name] || validateArgument(p, a.Value) != nil {
			return nil, ErrInvalid
		}
		seen[a.Name] = true
		values[a.Name] = clone(a.Value)
	}
	for name, p := range definitionsByName {
		if _, ok := values[name]; !ok && p.Required {
			return nil, ErrInvalid
		}
	}
	return values, nil
}
