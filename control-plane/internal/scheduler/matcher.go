package scheduler

import (
	"github.com/grid-computing/control-plane/internal/domain"
)

// matchScore computes a compatibility score between a task and a worker.
// Higher score = better match. Returns -1 if the pair is incompatible.
func matchScore(task *domain.Task, worker *domain.Worker) int {
	// Worker must support the task's AI agent
	if !hasAgent(worker.Agents, task.AIAgent) {
		return -1
	}

	// Worker must be idle
	if worker.State != domain.WorkerStateIdle {
		return -1
	}

	score := 0

	// Base score from task priority (1-10 → 0-100)
	score += task.Priority * 10

	// Bonus for high-capacity workers
	score += worker.CapacityScore / 10

	// Penalty for workers that have been assigned many retries
	score -= task.RetryCount * 5

	return score
}

// hasAgent returns true if the agent list contains the given agent id.
func hasAgent(agents []string, agent string) bool {
	for _, a := range agents {
		if a == agent {
			return true
		}
	}
	return false
}

// bestWorker selects the worker with the highest match score for a given task.
// Returns nil if no worker is compatible.
func bestWorker(task *domain.Task, workers []*domain.Worker) *domain.Worker {
	var best *domain.Worker
	bestScore := -1

	for _, w := range workers {
		score := matchScore(task, w)
		if score > bestScore {
			bestScore = score
			best = w
		}
	}

	return best
}
