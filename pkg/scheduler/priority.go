package scheduler

import (
	"slices"

	"github.com/absmach/propeller/pkg/task"
)

const defaultPriority = 50

func GetReadyTasksByPriority(tasks []task.Task) []task.Task {
	sorted := make([]task.Task, len(tasks))
	copy(sorted, tasks)

	slices.SortFunc(sorted, func(a, b task.Task) int {
		pa, pb := a.Priority, b.Priority
		if pa == 0 {
			pa = defaultPriority
		}
		if pb == 0 {
			pb = defaultPriority
		}

		if pa != pb {
			// Higher priority first.
			if pa > pb {
				return -1
			}

			return 1
		}

		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}

		return 0
	})

	return sorted
}
