package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteProfilesCreatesNonEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	err := writeProfiles(dir, func() error {
		buf := make([]byte, 0, 4096)
		for i := 0; i < 50_000; i++ {
			buf = append(buf, byte(i), byte(i>>8), byte(i>>16))
			if len(buf) > 2048 {
				buf = buf[:0]
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("writeProfiles: %v", err)
	}

	for _, name := range []string{"cpu.pprof", "alloc.pprof"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}
