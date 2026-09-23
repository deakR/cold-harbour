package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
)

const (
	envWriteProfile  = "WRITE_PROFILE"
	cpuProfileName   = "cpu.pprof"
	allocProfileName = "alloc.pprof"
)

func profileEnabled() bool {
	return os.Getenv(envWriteProfile) == "1"
}

func writeProfiles(dir string, work func() error) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	cpuPath := filepath.Join(dir, cpuProfileName)
	cpuFile, err := os.Create(cpuPath) //#nosec G304 -- profile directory is chosen by the load tool
	if err != nil {
		return fmt.Errorf("create cpu profile: %w", err)
	}
	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		_ = cpuFile.Close()
		return fmt.Errorf("start cpu profile: %w", err)
	}

	workErr := work()
	pprof.StopCPUProfile()
	if err := cpuFile.Close(); err != nil && workErr == nil {
		workErr = fmt.Errorf("close cpu profile: %w", err)
	}
	if workErr != nil {
		return workErr
	}

	runtime.GC()
	allocPath := filepath.Join(dir, allocProfileName)
	allocFile, err := os.Create(allocPath) //#nosec G304 -- profile directory is chosen by the load tool
	if err != nil {
		return fmt.Errorf("create alloc profile: %w", err)
	}
	defer allocFile.Close()

	if err := pprof.Lookup("allocs").WriteTo(allocFile, 0); err != nil {
		return fmt.Errorf("write alloc profile: %w", err)
	}
	return nil
}
