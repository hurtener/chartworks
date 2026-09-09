// Package httpmount defines the canonical mount shared by server configuration,
// public OpenAPI and clients. Mount metadata grants no identity or resource reach.
package httpmount

import "strings"

// Valid accepts the root mount or a bounded absolute path with canonical ASCII
// segments. Percent escapes, dot traversal, duplicate/trailing slashes and URL
// query/fragment syntax are excluded before any routing or credential forwarding.
func Valid(path string) bool {
	if path == "/" {
		return true
	}
	if len(path) < 2 || len(path) > 64 || path[0] != '/' {
		return false
	}
	for _, segment := range strings.Split(path[1:], "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
		for _, r := range segment {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.' && r != ':' {
				return false
			}
		}
	}
	return true
}
