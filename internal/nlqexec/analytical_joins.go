package nlqexec

import (
	"context"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

func rebaseAnalyticalExpression(e exec.AnalyticalExpression, from, to string) exec.AnalyticalExpression {
	rebase := func(name string) string {
		if name == "" {
			return ""
		}
		if strings.Contains(name, "/") {
			parts := strings.SplitN(name, "/", 2)
			return exec.AnalyticalColumnName(to, parts[0], parts[1])
		}
		return exec.AnalyticalColumnName(to, from, name)
	}
	e.Column = rebase(e.Column)
	for i := range e.Filters {
		e.Filters[i].Column = rebase(e.Filters[i].Column)
	}
	for i := range e.Args {
		e.Args[i] = rebaseAnalyticalExpression(e.Args[i], from, to)
	}
	return e
}

// compileAnalyticalJoins finds exactly one reviewed tree connecting the selected
// output relations. Unselected relationships cannot silently expand populations;
// ambiguous paths require review instead of a convenient model-selected join.
func compileAnalyticalJoins(ctx context.Context, a admission, c *exec.AnalyticalContract) error {
	needed := map[string]bool{c.Dataset: true}
	field := func(name string) {
		if p := strings.SplitN(name, "/", 2); len(p) == 2 {
			needed[p[0]] = true
		}
	}
	var visit func(exec.AnalyticalExpression)
	visit = func(e exec.AnalyticalExpression) {
		field(e.Column)
		for _, f := range e.Filters {
			field(f.Column)
		}
		for _, arg := range e.Args {
			visit(arg)
		}
	}
	for _, m := range c.Metrics {
		visit(m.Expression)
	}
	if c.Grain != nil {
		for _, name := range c.Grain.Columns {
			field(name)
		}
		for _, b := range c.Grain.Buckets {
			field(b.Column)
		}
	}
	if len(needed) == 1 {
		return nil
	}
	if len(needed) > 1 && (c.Grain == nil || len(c.Grain.Columns)+len(c.Grain.Buckets) == 0) && c.QueryPopulation == nil {
		if a.binding.Dialect != "postgres" {
			return analyticalUnsupported("analytical_shape_unsupported")
		}
		for id := range needed {
			c.Populations = append(c.Populations, id)
		}
		sort.Strings(c.Populations)
		return nil
	}
	if len(needed) > 4 {
		return exec.ErrLimit
	}
	var candidates []exec.AnalyticalJoin
	seen := map[string]bool{}
	for _, pub := range a.publications {
		def := pub.Definition
		for _, j := range def.Joins {
			if err := ctx.Err(); err != nil {
				return err
			}
			selected := len(a.route.Request.JoinChoices) == 0
			for _, pick := range a.route.Request.JoinChoices {
				if pick.Topic == def.Topic && pick.JoinID == j.ID {
					selected = true
				}
			}
			if !selected {
				continue
			}
			compiler := analyticalCompiler{ctx: ctx, definition: def, binding: a.binding, joins: true}
			lc, err := compiler.column(j.Left)
			if err != nil {
				return err
			}
			compiler.dataset = ""
			rc, err := compiler.column(j.Right)
			if err != nil {
				return err
			}
			x := exec.AnalyticalJoin{Left: j.Left.Dataset, Right: j.Right.Dataset, Type: string(j.Type), LeftColumns: []string{lc.SourceName}, RightColumns: []string{rc.SourceName}}
			if j.Type == semantics.JoinInner && x.Left > x.Right {
				x.Left, x.Right = x.Right, x.Left
				x.LeftColumns, x.RightColumns = x.RightColumns, x.LeftColumns
			}
			key := exec.Hash(x)
			if !seen[key] {
				candidates = append(candidates, x)
				seen[key] = true
			}
		}
	}
	if len(candidates) > 8 {
		return exec.ErrLimit
	}
	sort.Slice(candidates, func(i, j int) bool { return exec.Hash(candidates[i]) < exec.Hash(candidates[j]) })
	// Enumerate bounded subsets; each chosen edge must add exactly one relation.
	var solutions [][]exec.AnalyticalJoin
	for mask := 1; mask < (1 << len(candidates)); mask++ {
		chosen := []exec.AnalyticalJoin{}
		for i, j := range candidates {
			if mask&(1<<i) != 0 {
				chosen = append(chosen, j)
			}
		}
		if len(chosen) > 3 {
			continue
		}
		reached := map[string]bool{c.Dataset: true}
		ordered := []exec.AnalyticalJoin{}
		used := map[int]bool{}
		for len(ordered) < len(chosen) {
			advanced := false
			for i, j := range chosen {
				if used[i] || reached[j.Left] == reached[j.Right] {
					continue
				}
				reached[j.Left], reached[j.Right] = true, true
				ordered = append(ordered, j)
				used[i] = true
				advanced = true
			}
			if !advanced {
				break
			}
		}
		if len(ordered) != len(chosen) {
			continue
		}
		complete := true
		for id := range needed {
			complete = complete && reached[id]
		}
		if !complete {
			continue
		}
		// Reject gratuitous leaves that affect population without serving an output.
		degree := map[string]int{}
		for _, j := range ordered {
			degree[j.Left]++
			degree[j.Right]++
		}
		for id, n := range degree {
			if n == 1 && !needed[id] {
				complete = false
			}
		}
		if complete {
			solutions = append(solutions, ordered)
		}
	}
	if len(solutions) != 1 {
		return analyticalUnsupported("analytical_join_ambiguous")
	}
	c.Joins = solutions[0]
	return exec.ValidateAnalyticalJoins(*c, a.binding)
}

func analyticalJoinGuidance(c *exec.AnalyticalContract) string {
	if c != nil && len(c.Populations) > 0 {
		return " Compute each selected source population in its own single-row aggregate CTE or derived table, with exactly its reviewed metric filters and named outputs. Combine those singleton rows by CROSS JOIN and apply only the reviewed arithmetic at the outer SELECT. Do not join raw fact rows, add grouping, coalesce empty aggregates, or substitute extra population filters."
	}
	if c == nil || len(c.Joins) == 0 {
		return ""
	}
	return " Use exactly the reviewed equality relationships in a left-deep join tree. Preserve each reviewed INNER versus LEFT join and its complete keys. Physical source keys have been checked for the selected aggregate inputs; do not preaggregate, duplicate, add, omit or rearrange outer joins. Qualify joined columns by their relation alias. Reviewed relation and column names are in the authorized semantic context."
}
