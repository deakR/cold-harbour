package main

import (
	"os"
	"path/filepath"
	"testing"

	"coldharbour/internal/detect"
)

func TestScanFileDetectsPII(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")
	content := "User alice registered with email alice@example.com and phone (555) 123-4567\nNormal line without secrets\n"
	if err := os.WriteFile(logFile, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	kinds := []detect.Kind{detect.KindEmail, detect.KindPhoneUS}
	findings, lines, err := scanFile(logFile, kinds)
	if err != nil {
		t.Fatalf("scanFile error: %v", err)
	}
	if lines != 2 {
		t.Errorf("expected 2 lines, got %d", lines)
	}
	if findings[string(detect.KindEmail)] != 1 {
		t.Errorf("expected 1 email, got %d", findings[string(detect.KindEmail)])
	}
	if findings[string(detect.KindPhoneUS)] != 1 {
		t.Errorf("expected 1 phone, got %d", findings[string(detect.KindPhoneUS)])
	}
}
