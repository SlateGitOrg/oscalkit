package oscal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPinnedContentChecksums verifies that the committed NIST content (some
// of it gzip-compressed) decompresses to exactly the upstream bytes recorded
// in SHA256SUMS by scripts/fetch-nist-content.sh.
func TestPinnedContentChecksums(t *testing.T) {
	sums, err := os.Open(filepath.Join(nistDir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sums.Close() }()
	n := 0
	sc := bufio.NewScanner(sums)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			t.Fatalf("bad line %q", sc.Text())
		}
		want, name := fields[0], fields[1]
		raw, err := readMaybeGz(filepath.Join(nistDir, name))
		if err != nil {
			t.Fatal(err)
		}
		got := sha256.Sum256(raw)
		if hex.EncodeToString(got[:]) != want {
			t.Errorf("%s: checksum mismatch", name)
		}
		n++
	}
	if n != 9 {
		t.Fatalf("expected 9 pinned files, found %d", n)
	}
}

func readMaybeGz(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		f, err = os.Open(path + ".gz")
		if err != nil {
			return nil, err
		}
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return Normalize(data, path)
}
