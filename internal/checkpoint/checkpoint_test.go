package checkpoint

import (
	"errors"
	"testing"

	"coldharbour/internal/redact"
)

var demoJobs = []struct {
	ID    string
	Input string
}{
	{ID: "job-5", Input: redact.M1Fixture},
	{ID: "job-1", Input: "Email alice.nguyen@school.edu about the lab report."},
	{ID: "job-4", Input: "Call 415-555-0199 before noon."},
	{ID: "job-2", Input: "Employee SSN 987-65-4321 is on the form."},
	{ID: "job-3", Input: "Reach +1 212 555 0100 or bob.smith@mail.net."},
}

func TestRunCrashResumeEqualsRedactPII(t *testing.T) {
	store := NewCheckpointStore()
	jobID := "job-5"

	_, err := store.Run(jobID, redact.M1Fixture, CrashAfterStep1)
	if !errors.Is(err, ErrSimulatedCrash) {
		t.Fatalf("Run err = %v, want ErrSimulatedCrash", err)
	}
	if store.Step1Passes(jobID) != 1 {
		t.Fatalf("Step1Passes after crash = %d, want 1", store.Step1Passes(jobID))
	}

	got, err := store.Resume(jobID)
	if err != nil {
		t.Fatal(err)
	}
	want, err := redact.RedactPII(redact.M1Fixture)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Resume = %+v, want %+v", got, want)
	}
	if store.Step1Passes(jobID) != 1 {
		t.Fatalf("Step1Passes after Resume = %d, want 1", store.Step1Passes(jobID))
	}

	again, err := store.Resume(jobID)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("second Resume = %+v, want %+v", again, got)
	}
	if store.Step1Passes(jobID) != 1 {
		t.Fatalf("Step1Passes after second Resume = %d, want 1", store.Step1Passes(jobID))
	}
}

func TestRunCrashNeverMatchesRedactPII(t *testing.T) {
	store := NewCheckpointStore()
	for _, job := range demoJobs {
		got, err := store.Run(job.ID, job.Input, CrashNever)
		if err != nil {
			t.Fatalf("%s: Run err = %v", job.ID, err)
		}
		want, err := redact.RedactPII(job.Input)
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
	_, err := store.Run("", "x", CrashNever)
	if !errors.Is(err, ErrEmptyJobID) {
		t.Fatalf("empty id err = %v, want ErrEmptyJobID", err)
	}

	if _, err := store.Run("job-1", "x", CrashNever); err != nil {
		t.Fatal(err)
	}
	_, err = store.Run("job-1", "x", CrashNever)
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
