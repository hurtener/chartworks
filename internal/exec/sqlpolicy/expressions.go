package sqlpolicy

// ExpressionSignature describes native expression forms, distinct from ordinary
// callable names. It never grants a function-name or namespace permission.
func ExpressionSignature(dialect, name string) (Signature, bool) {
	if dialect != "postgres" {
		return Signature{}, false
	}
	s := Signature{Kind: "special", Result: "same"}
	switch name {
	case "coalesce", "greatest", "least":
		s.Arguments = []string{"any", "same?"}
		s.Variadic = true
	case "nullif":
		s.Arguments = []string{"any", "same"}
	default:
		return Signature{}, false
	}
	return s, true
}

func AllowsExpression(dialect, name string, arguments []string) bool {
	s, ok := ExpressionSignature(dialect, name)
	if !ok {
		return false
	}
	return allowsTypedCall(dialect, name, s, arguments, false, false, false, false, false, false)
}
