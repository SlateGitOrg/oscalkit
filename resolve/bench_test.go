package resolve

import (
	"context"
	"path/filepath"
	"testing"
)

// BenchmarkResolveNISTHigh measures resolving the official HIGH baseline
// (370 controls) against the full SP 800-53 rev5 catalog, including reading
// and decompressing the catalog from disk on every iteration.
func BenchmarkResolveNISTHigh(b *testing.B) {
	profile := filepath.Join(nistDir, "NIST_SP-800-53_rev5_HIGH-baseline_profile.json")
	for b.Loop() {
		if _, err := ResolveFile(context.Background(), profile, fixedOptions()); err != nil {
			b.Fatal(err)
		}
	}
}
