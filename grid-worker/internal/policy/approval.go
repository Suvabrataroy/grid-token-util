package policy

import (
	"sync"
	"time"
)

const defaultApprovalTTL = 30 * time.Minute

// Approval represents a pending manual approval request.
type Approval struct {
	TaskID      string
	RequestedAt time.Time
	ExpiresAt   time.Time
}

// ApprovalStore manages pending and approved tasks for manual-mode execution.
type ApprovalStore struct {
	mu       sync.RWMutex
	pending  map[string]*Approval // taskID → approval
	approved map[string]bool
}

// NewApprovalStore creates a new ApprovalStore.
func NewApprovalStore() *ApprovalStore {
	return &ApprovalStore{
		pending:  make(map[string]*Approval),
		approved: make(map[string]bool),
	}
}

// Request creates and stores a new approval request for the given task ID.
// If a request for the task already exists, it is returned as-is.
func (s *ApprovalStore) Request(taskID string) *Approval {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.pending[taskID]; ok {
		return existing
	}

	now := time.Now()
	approval := &Approval{
		TaskID:      taskID,
		RequestedAt: now,
		ExpiresAt:   now.Add(defaultApprovalTTL),
	}
	s.pending[taskID] = approval
	return approval
}

// Approve marks a pending task as approved.
// Returns false if the task is not in the pending list.
func (s *ApprovalStore) Approve(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	approval, ok := s.pending[taskID]
	if !ok {
		return false
	}

	// Check if expired
	if time.Now().After(approval.ExpiresAt) {
		delete(s.pending, taskID)
		return false
	}

	delete(s.pending, taskID)
	s.approved[taskID] = true
	return true
}

// Deny removes a pending approval request without approving it.
// Returns false if the task is not in the pending list.
func (s *ApprovalStore) Deny(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.pending[taskID]
	if !ok {
		return false
	}

	delete(s.pending, taskID)
	delete(s.approved, taskID)
	return true
}

// IsApproved returns true if the task has been approved and the approval has not been consumed.
func (s *ApprovalStore) IsApproved(taskID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.approved[taskID]
}

// ConsumeApproval removes the approval for a task after it has been acted upon.
func (s *ApprovalStore) ConsumeApproval(taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.approved, taskID)
}

// PendingList returns a snapshot of all unexpired pending approval requests.
func (s *ApprovalStore) PendingList() []*Approval {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make([]*Approval, 0, len(s.pending))
	for _, a := range s.pending {
		if now.Before(a.ExpiresAt) {
			// Return a copy to avoid data races
			copy := *a
			result = append(result, &copy)
		}
	}
	return result
}

// PurgeExpired removes all expired approval requests. Should be called periodically.
func (s *ApprovalStore) PurgeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, a := range s.pending {
		if now.After(a.ExpiresAt) {
			delete(s.pending, id)
		}
	}
}
