// Package signatureparser exposes structural evidence from the same pinned
// parser as warehouse safety inspection. Evidence never authorizes a read.
package signatureparser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// Process-wide admission bounds the native worker stacks even when separate
// validators or analytical consumers concurrently request structural evidence.
// The fixed two-slot budget is shared; each call owns/releases its own lease.
var nativeSlots = make(chan struct{}, 2)

var ErrSyntax = errors.New("signatureparser: unsupported call syntax")

func Inspect(ctx context.Context, query, dialect string, nodes, depth int) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(query) == 0 || len(query) > 256<<10 || strings.ContainsRune(query, 0) || strings.ContainsRune(dialect, 0) || nodes < 1 || nodes > 100000 || depth < 1 || depth > 64 {
		return nil, ErrSyntax
	}
	select {
	case nativeSlots <- struct{}{}:
		defer func() { <-nativeSlots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := inspect(query, dialect, nodes, depth)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, ErrSyntax
	}
	var root map[string]any
	if json.Unmarshal([]byte(raw), &root) != nil || len(root) != 1 || root["error"] != nil {
		return nil, ErrSyntax
	}
	return root, nil
}
