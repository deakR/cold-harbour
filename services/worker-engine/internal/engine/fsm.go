package engine

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"coldharbor/worker-engine/internal/model"
)

// EventPublisher defines the interface for broadcasting state transitions
type EventPublisher interface {
	Publish(ctx context.Context, event *model.EventMessage) error
}

// FSM manages and enforces finite state machine transitions for a compartment
type FSM struct {
	compartmentID string
	workerID      string
	currentState  model.State
	mu            sync.RWMutex
	publisher     EventPublisher
}

// NewFSM initializes a new state machine in the specified initial state
func NewFSM(compartmentID, workerID string, initial model.State, publisher EventPublisher) *FSM {
	return &FSM{
		compartmentID: compartmentID,
		workerID:      workerID,
		currentState:  initial,
		publisher:     publisher,
	}
}

// CurrentState returns the current state thread-safely
func (f *FSM) CurrentState() model.State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.currentState
}

// CanTransition checks whether moving to the desired state is valid
func (f *FSM) CanTransition(to model.State) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return model.ValidateTransition(f.currentState, to) == nil
}

// Transition moves the compartment to the new state, validating the transition and broadcasting an event
func (f *FSM) Transition(ctx context.Context, to model.State, progress int, details string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	from := f.currentState
	if err := model.ValidateTransition(from, to); err != nil {
		return fmt.Errorf("fsm transition rejected: %w", err)
	}

	if f.publisher != nil {
		event := &model.EventMessage{
			CompartmentID: f.compartmentID,
			WorkerID:      f.workerID,
			FromState:     from,
			ToState:       to,
			PreviousState: from,
			CurrentState:  to,
			CheckpointPct: progress,
			Progress:      progress,
			Timestamp:     time.Now().UTC(),
			Details:       details,
		}
		if err := f.publisher.Publish(ctx, event); err != nil {
			log.Printf("[fsm] failed to publish %s -> %s for compartment %s: %v",
				from, to, f.compartmentID, err)
			return fmt.Errorf("persist transition event: %w", err)
		}
	}

	f.currentState = to
	return nil
}
