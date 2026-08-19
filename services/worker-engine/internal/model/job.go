package model

import "time"

// JobMessage represents the payload dispatched via Redis Stream (coldharbor:jobs)
type JobMessage struct {
	CompartmentID string         `json:"compartmentId"`
	Context       string         `json:"context"` // INNIE, OUTIE, SYSTEM, ADMIN
	OwnerID       string         `json:"ownerId"`
	TaskType      string         `json:"taskType"`
	Payload       map[string]any `json:"payload"`
	CreatedAt     time.Time      `json:"createdAt"`
}

// EventMessage represents a live state transition broadcast via Redis Pub/Sub (coldharbor:events)
type EventMessage struct {
	EventID       string    `json:"eventId"`
	CompartmentID string    `json:"compartmentId"`
	WorkerID      string    `json:"workerId"`
	FromState     State     `json:"fromState"`
	ToState       State     `json:"toState"`
	CheckpointPct int       `json:"checkpointPct"`
	Timestamp     time.Time `json:"timestamp"`
	Details       string    `json:"details"`
}

// DeadDropPayload represents the sealed immutable result stored in Redis String with TTL
type DeadDropPayload struct {
	CompartmentID string         `json:"compartmentId"`
	OwnerID       string         `json:"ownerId"`
	Context       string         `json:"context"`
	TaskType      string         `json:"taskType"`
	Output        map[string]any `json:"output"`
	Checksum      string         `json:"checksum"` // SHA-256 hex string
	ArchivedAt    time.Time      `json:"archivedAt"`
	TTLSeconds    int            `json:"ttlSeconds"`
}

// HeartbeatPayload represents worker liveness emitted to worker:{workerId}:heartbeat
type HeartbeatPayload struct {
	WorkerID            string    `json:"workerId"`
	Status              string    `json:"status"` // IDLE, BUSY
	ActiveCompartmentID string    `json:"activeCompartmentId,omitempty"`
	Timestamp           time.Time `json:"timestamp"`
}