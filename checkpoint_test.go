package main

import (
	"errors"
	"testing"
)

func TestRunCrashResumeEqualsRedactPII(t *testing.T) {
	store := NewCheckpointStore()
	job := Job{ID: "job-5", Input: m1Fixture}

	_, err := store.Run(job, CrashAfterStep1)
	if !errors.Is(err, ErrSimulatedCrash) {
		t.Fatalf("Run err = %v, want ErrSimulatedCrash", err)
	}
	if store.Step1Passes(job.ID) != 1 {
		t.Fatalf("Step1Passes after crash = %d, want 1", store.Step1Passes(job.ID))
	}

	got, err := store.Resume(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	want, err := RedactPII(job.Input)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Resume = %+v, want %+v", got, want)
	}
	if store.Step1Passes(job.ID) != 1 {
		t.Fatalf("Step1Passes after Resume = %d, want 1", store.Step1Passes(job.ID))
	}

	again, err := store.Resume(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("second Resume = %+v, want %+v", again, got)
	}
	if store.Step1Passes(job.ID) != 1 {
		t.Fatalf("Step1Passes after second Resume = %d, want 1", store.Step1Passes(job.ID))
	}
}

func TestRunCrashNeverMatchesRedactPII(t *testing.T) {
	store := NewCheckpointStore()
	for _, job := range demoJobs {
		got, err := store.Run(job, CrashNever)
		if err != nil {
			t.Fatalf("%s: Run err = %v", job.ID, err)
		}
		want, err := RedactPII(job.Input)
		if err != nil {
			t.Fatalf("%s: RedactPII err = %v", job.ID, err)
		}
		if got != want {
			t.Fatalf("%s: Run = %+v, want %+v", job.ID, got, want)
		}
		if store.Step1Passes(job.ID) != 1 {
			t.Fatalf("%s: Step1Passes = %d, want 1", job.ID, store.Step1Passes(job.ID))
		}
	}
}

func TestRunRejectsEmptyAndExisting(t *testing.T) {
	store := NewCheckpointStore()
	_, err := store.Run(Job{Input: "x"}, CrashNever)
	if !errors.Is(err, ErrEmptyJobID) {
		t.Fatalf("empty id err = %v, want ErrEmptyJobID", err)
	}

	job := Job{ID: "job-1", Input: "x"}
	if _, err := store.Run(job, CrashNever); err != nil {
		t.Fatal(err)
	}
	_, err = store.Run(job, CrashNever)
	if !errors.Is(err, ErrJobExists) {
		t.Fatalf("duplicate err = %v, want ErrJobExists", err)
	}
}

func TestResumeRejectsEmptyAndMissing(t *testing.T) {
	store := NewCheckpointStore()
	_, err := store.Resume("")
	if !errors.Is(err, ErrEmptyJobID) {
		t.Fatalf("empty id err = %v, want ErrEmptyJobID", err)
	}
	_, err = store.Resume("missing")
	if !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("missing err = %v, want ErrUnknownJob", err)
	}
}
