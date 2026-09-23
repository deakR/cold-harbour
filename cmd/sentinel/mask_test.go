package main

import (
	"strings"
	"testing"
)

func BenchmarkMaskLine(b *testing.B) {
	base := "contact jane.doe@example.com "
	line := base + strings.Repeat("x", 1024-len(base))
	if len(line) != 1024 {
		b.Fatalf("line length = %d, want 1024", len(line))
	}
	b.SetBytes(int64(len(line)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		maskLine(line, 65536, false)
	}
}
