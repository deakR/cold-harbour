package main

import (
	"context"

	"batchseal/backend"
)

// App exposes BatchSeal operations to the frontend.
type App struct {
	ctx    context.Context
	client *backend.Client
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{client: backend.NewClient(backend.LoadConfig())}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetConfig returns current connection settings.
func (a *App) GetConfig() backend.Config {
	return backend.LoadConfig()
}

// SaveConfig persists connection settings and rebuilds the client.
func (a *App) SaveConfig(cfg backend.Config) error {
	if err := backend.SaveConfig(cfg); err != nil {
		return err
	}
	a.client = backend.NewClient(cfg)
	return nil
}

// DispatchBatch creates one compartment batch from numeric values.
func (a *App) DispatchBatch(taskType string, values []float64) (string, error) {
	return a.client.Dispatch(taskType, values)
}

// DispatchChaos creates a batch whose worker crashes at step 2,
// demonstrating PEL checkpoint recovery.
func (a *App) DispatchChaos(taskType string, values []float64) (string, error) {
	in := make([]any, len(values))
	for i, v := range values {
		in[i] = v
	}
	return a.client.DispatchRaw(taskType, map[string]any{
		"batchSize":           len(values),
		"inputValues":         in,
		"simulateCrashAtStep": 2,
	})
}

// BatchStatus polls one compartment.
func (a *App) BatchStatus(id string) (backend.Compartment, error) {
	return a.client.Status(id)
}

// BatchReceipt fetches a dead drop, verifies the seal, saves a local copy.
func (a *App) BatchReceipt(id string) (backend.Receipt, error) {
	return a.client.Receipt(id)
}

// AuditHistory lists durable history rows for an owner (empty = all).
func (a *App) AuditHistory(owner string) ([]backend.Audit, error) {
	return a.client.Audits(owner)
}

// LocalReceipts lists offline-saved sealed copies.
func (a *App) LocalReceipts() ([]backend.Receipt, error) {
	return backend.ListReceipts()
}

// Fleet returns worker liveness.
func (a *App) Fleet() ([]backend.Worker, error) {
	return a.client.Workers()
}

// DeadLetters returns the DLQ depth.
func (a *App) DeadLetters() (int, error) {
	return a.client.DLQSize()
}

// RedriveDLQ requeues dead-letter entries for another attempt.
func (a *App) RedriveDLQ(limit int) (int, error) {
	return a.client.Redrive(limit)
}
