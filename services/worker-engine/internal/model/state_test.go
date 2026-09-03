package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestValidateTransition(t *testing.T) {
	validCases := []struct {
		from State
		to   State
	}{
		{StateCreated, StateQueued},
		{StateCreated, StateFailed},
		{StateQueued, StateRunning},
		{StateQueued, StateFailed},
		{StateRunning, StateCheckpoint},
		{StateRunning, StateCompleted},
		{StateRunning, StateFailed},
		{StateCheckpoint, StateRunning},
		{StateCheckpoint, StateCompleted},
		{StateCheckpoint, StateFailed},
		{StateCompleted, StateArchived},
		{StateCompleted, StateFailed},
		{StateArchived, StatePurged},
		{StateFailed, StatePurged},
	}

	for _, tc := range validCases {
		if err := ValidateTransition(tc.from, tc.to); err != nil {
			t.Errorf("expected transition %s -> %s to be valid, got: %v", tc.from, tc.to, err)
		}
	}

	invalidCases := []struct {
		from State
		to   State
	}{
		{StatePurged, StateRunning},
		{StatePurged, StateCreated},
		{StateRunning, StateArchived},
		{StateRunning, StatePurged},
		{StateCreated, StateCompleted},
		{StateCompleted, StateRunning},
		{StateArchived, StateRunning},
	}

	for _, tc := range invalidCases {
		if err := ValidateTransition(tc.from, tc.to); err == nil {
			t.Errorf("expected transition %s -> %s to be invalid, got nil", tc.from, tc.to)
		}
	}
}

func TestModelJSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)

	job := JobMessage{
		CompartmentID: "cpt-123",
		Context:       "INNIE",
		OwnerID:       "usr-456",
		TaskType:      "DATA_REDUCTION",
		Payload:       map[string]any{"batchSize": float64(500)},
		CreatedAt:     now,
	}
	jobBytes, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("failed to marshal JobMessage: %v", err)
	}

	var parsedJob JobMessage
	if err := json.Unmarshal(jobBytes, &parsedJob); err != nil {
		t.Fatalf("failed to unmarshal JobMessage: %v", err)
	}
	if parsedJob.CompartmentID != "cpt-123" || parsedJob.Context != "INNIE" {
		t.Errorf("mismatched unmarshaled job: %+v", parsedJob)
	}

	event := EventMessage{
		EventID:       "evt-789",
		CompartmentID: "cpt-123",
		WorkerID:      "worker-go-01",
		FromState:     StateRunning,
		ToState:       StateCheckpoint,
		CheckpointPct: 50,
		Timestamp:     now,
		Details:       "test checkpoint",
	}
	eventBytes, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal EventMessage: %v", err)
	}
	var parsedEvent EventMessage
	if err := json.Unmarshal(eventBytes, &parsedEvent); err != nil {
		t.Fatalf("failed to unmarshal EventMessage: %v", err)
	}
	if parsedEvent.CheckpointPct != 50 || parsedEvent.ToState != StateCheckpoint {
		t.Errorf("mismatched unmarshaled event: %+v", parsedEvent)
	}

	deadDrop := DeadDropPayload{
		CompartmentID: "cpt-123",
		OwnerID:       "usr-456",
		Context:       "INNIE",
		TaskType:      "DATA_REDUCTION",
		Output:        map[string]any{"status": "SUCCESS"},
		Checksum:      "abcdef0123456789",
		ArchivedAt:    now,
		TTLSeconds:    3600,
	}
	ddBytes, err := json.Marshal(deadDrop)
	if err != nil {
		t.Fatalf("failed to marshal DeadDropPayload: %v", err)
	}
	var parsedDD DeadDropPayload
	if err := json.Unmarshal(ddBytes, &parsedDD); err != nil {
		t.Fatalf("failed to unmarshal DeadDropPayload: %v", err)
	}
	if parsedDD.Checksum != "abcdef0123456789" || parsedDD.TTLSeconds != 3600 {
		t.Errorf("mismatched unmarshaled dead drop: %+v", parsedDD)
	}

	heartbeat := HeartbeatPayload{
		WorkerID:            "worker-go-01",
		Status:              "BUSY",
		ActiveCompartmentID: "cpt-123",
		Timestamp:           now,
	}
	hbBytes, err := json.Marshal(heartbeat)
	if err != nil {
		t.Fatalf("failed to marshal HeartbeatPayload: %v", err)
	}
	var parsedHB HeartbeatPayload
	if err := json.Unmarshal(hbBytes, &parsedHB); err != nil {
		t.Fatalf("failed to unmarshal HeartbeatPayload: %v", err)
	}
	if parsedHB.Status != "BUSY" || parsedHB.ActiveCompartmentID != "cpt-123" {
		t.Errorf("mismatched unmarshaled heartbeat: %+v", parsedHB)
	}
}
