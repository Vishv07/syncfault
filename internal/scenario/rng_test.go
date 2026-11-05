package scenario

import (
	"bytes"
	"testing"
)

func TestStreamDeterminism(t *testing.T) {
	s1 := NewStream(412)
	s2 := NewStream(412)

	for i := 0; i < 100; i++ {
		if a, b := s1.IntRange(10, 5000), s2.IntRange(10, 5000); a != b {
			t.Fatalf("mismatch at step %d: %d != %d", i, a, b)
		}
	}

	c1 := ContentBytes(412, 12, 8197)
	c2 := ContentBytes(412, 12, 8197)
	if !bytes.Equal(c1, c2) {
		t.Fatal("expected identical generated content for same (seed, opIndex, size)")
	}
	c3 := ContentBytes(412, 13, 8197)
	if bytes.Equal(c1, c3) {
		t.Fatal("expected different content for different opIndex")
	}
}
