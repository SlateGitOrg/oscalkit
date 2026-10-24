package oscal

import (
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 4122 version-5 UUIDs are defined over SHA-1.
	"fmt"
)

// UUIDNamespace is the namespace used for oscalkit's deterministic
// (version 5) UUIDs.
var UUIDNamespace = [16]byte{
	0x6f, 0x73, 0x63, 0x61, 0x6c, 0x6b, 0x69, 0x74,
	0x2d, 0x75, 0x75, 0x69, 0x64, 0x2d, 0x6e, 0x73,
}

// UUIDGen produces UUIDs for generated documents.
type UUIDGen interface {
	// New returns a UUID. key describes the object being identified; a
	// deterministic generator derives the UUID from it.
	New(key string) string
}

// RandomUUIDs generates random RFC 4122 version-4 UUIDs.
type RandomUUIDs struct{}

// New returns a random version-4 UUID.
func (RandomUUIDs) New(string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("oscal: crypto/rand failed: %v", err))
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return formatUUID(b)
}

// DeterministicUUIDs generates RFC 4122 version-5 UUIDs from a seed and the
// object key, so repeated runs produce byte-identical documents.
type DeterministicUUIDs struct {
	Seed string
}

// New returns the version-5 UUID for seed+key.
func (d DeterministicUUIDs) New(key string) string {
	h := sha1.New() //nolint:gosec // see import comment.
	h.Write(UUIDNamespace[:])
	h.Write([]byte(d.Seed))
	h.Write([]byte{0})
	h.Write([]byte(key))
	sum := h.Sum(nil)
	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = b[6]&0x0f | 0x50
	b[8] = b[8]&0x3f | 0x80
	return formatUUID(b)
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
