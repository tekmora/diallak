//go:build semgrep_fixture

package fixture

import (
	"fmt"
	// ruleid: unsafe-requires-safety-proof
	"unsafe"
)

func view(b []byte) string {
	// ruleid: unsafe-requires-safety-proof
	return unsafe.String(&b[0], len(b))
}

func viewProved(b []byte) string {
	// SAFETY: b is non-empty and never written after this call, so the
	// string shares memory that does not change.
	// ok: unsafe-requires-safety-proof
	return unsafe.String(&b[0], len(b))
}

func viewProofTooFar(b []byte) string {
	// SAFETY: separated from the code by a blank line, so it does not count.

	// ruleid: unsafe-requires-safety-proof
	return unsafe.String(&b[0], len(b))
}

func name() string {
	// ok: unsafe-requires-safety-proof
	return fmt.Sprint("unsafe")
}
