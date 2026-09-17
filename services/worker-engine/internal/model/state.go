package model

import "fmt"

// State represents the lifecycle state of a compartment
type State string

const (
	StateCreated    State = "CREATED"
	StateQueued     State = "QUEUED"
	StateRunning    State = "RUNNING"
	StateCheckpoint State = "CHECKPOINT"
	StateCompleted  State = "COMPLETED"
	StateArchived   State = "ARCHIVED"
	StatePurged     State = "PURGED"
	StateFailed     State = "FAILED"
)

// validTransitions defines strict allowed state transitions
var validTransitions = map[State]map[State]bool{
	StateCreated: {
		StateQueued: true,
		StateFailed: true,
	},
	StateQueued: {
		StateRunning: true,
		StateFailed:  true,
	},
	StateRunning: {
		StateCheckpoint: true,
		StateCompleted:  true,
		StateFailed:     true,
	},
	StateCheckpoint: {
		StateRunning:   true,
		StateCompleted: true,
		StateFailed:    true,
	},
	StateCompleted: {
		StateArchived: true,
		StateFailed:   true,
	},
	StateArchived: {
		StatePurged: true,
	},
	StateFailed: {
		StatePurged: true,
	},
	StatePurged: {}, // Terminal state: no further transitions allowed
}

// ValidateTransition checks if moving from 'from' to 'to' is allowed
func ValidateTransition(from, to State) error {
	allowedNext, exists := validTransitions[from]
	if !exists || !allowedNext[to] {
		return fmt.Errorf("illegal state transition: cannot move from %s to %s", from, to)
	}
	return nil
}
