package exec

// unionNode preserves the exact set operator and both ordered operands. The
// grouped proof independently establishes that a deduplicated key spine is
// complete; normalization neither removes clauses nor authorizes UNION ALL.
func (n *warehouseAnalyticalNormalizer) unionNode(u map[string]any) (map[string]any, error) {
	if err := n.ctx.Err(); err != nil {
		return nil, err
	}
	if !warehouseAnalyticalFields(u, "left", "right", "all", "distinct") {
		return n.unsupported()
	}
	n.queryDepth++
	defer func() { n.queryDepth-- }()
	if n.queryDepth > 8 {
		return nil, ErrLimit
	}
	left, err := n.selectNode(object(u["left"]))
	if err != nil {
		return nil, err
	}
	right, err := n.selectNode(object(u["right"]))
	if err != nil {
		return nil, err
	}
	return map[string]any{"op": "SETOP_UNION", "all": truth(u["all"]), "larg": left, "rarg": right}, nil
}
