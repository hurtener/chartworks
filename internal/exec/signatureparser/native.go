//go:build cgo && (linux || darwin)

package signatureparser

/*
#cgo LDFLAGS: -lchartworks_signatures -ldl -lpthread -lm
#include <stdlib.h>
char *chartworks_signature_ast(const char*, const char*, size_t, size_t);
void chartworks_signature_free(char*);
const char* chartworks_signature_contract();
*/
import "C"
import (
	_ "embed"
	"unsafe"
)

// Embedding the exact contract makes Rust source changes invalidate Go's build
// cache too. Comparing the archive contract rejects stale native-cache contents.
//
//go:embed rustffi/Cargo.toml
var nativeManifest string

//go:embed rustffi/Cargo.lock
var nativeLock string

//go:embed rustffi/src/lib.rs
var nativeSource string

func contractMatches() bool {
	p := C.chartworks_signature_contract()
	return p != nil && C.GoString(p) == nativeManifest+nativeLock+nativeSource
}

func inspect(query, dialect string, nodes, depth int) (string, error) {
	if !contractMatches() {
		return "", ErrSyntax
	}
	q, d := C.CString(query), C.CString(dialect)
	defer C.free(unsafe.Pointer(q))
	defer C.free(unsafe.Pointer(d))
	result := C.chartworks_signature_ast(q, d, C.size_t(nodes), C.size_t(depth))
	if result == nil {
		return "", ErrSyntax
	}
	defer C.chartworks_signature_free(result)
	return C.GoString(result), nil
}
