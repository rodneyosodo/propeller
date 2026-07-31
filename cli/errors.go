package cli

import (
	"errors"
)

var (
	errNameRequired         = errors.New("name is required")
	errTasksRequired        = errors.New("at least one task is required")
	errTasksConflict        = errors.New("use either --tasks or --tasks-file, not both")
	errExperimentIDRequired = errors.New("experiment id is required")
	errRoundIDRequired      = errors.New("round id is required")
)
