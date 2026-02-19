package job

import "time"

// Job stores metadata for a logical grouping of tasks.
type Job struct {
	ID            string    `json:"id" db:"id"`
	Name          string    `json:"name" db:"name"`
	ExecutionMode string    `json:"execution_mode" db:"execution_mode"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}
