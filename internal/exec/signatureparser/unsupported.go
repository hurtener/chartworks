//go:build !cgo || (!linux && !darwin)

package signatureparser

func inspect(query, dialect string, nodes, depth int) (string, error) { return "", ErrSyntax }
