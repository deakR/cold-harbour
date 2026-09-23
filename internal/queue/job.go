package queue

type Job struct {
	ID       string
	Input    string
	TenantID string
	JobType  string
}
