package nlqexec

import (
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"go/ast"
)

func (c *analyticalCompiler) nullPolicyCall(n *ast.CallExpr, inputs map[string]semantics.Reference, used map[string]bool, filters []semantics.SemanticFilter, depth int) (exec.AnalyticalExpression, error) {
	fn, ok := n.Fun.(*ast.Ident)
	if !ok || fn.Name != "coalesce" || n.Ellipsis != 0 || len(n.Args) < 2 || len(n.Args) > 8 {
		return exec.AnalyticalExpression{}, analyticalUnsupported("analytical_expression_unsupported")
	}
	out := exec.AnalyticalExpression{Op: "coalesce"}
	for _, arg := range n.Args {
		x, err := c.expression(arg, inputs, used, filters, depth+1)
		if err != nil {
			return exec.AnalyticalExpression{}, err
		}
		out.Args = append(out.Args, x)
	}
	return out, nil
}
