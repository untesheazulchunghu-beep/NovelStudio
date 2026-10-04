package task

import "sync"

// UserQuota tracks per-user concurrency limits to prevent any single user from
// monopolizing the global task semaphore.
type UserQuota struct {
	mu      sync.Mutex
	running map[string]int // userID -> running task count
	limit   int            // max concurrent tasks per user
}

func NewUserQuota(limit int) *UserQuota {
	if limit < 1 {
		limit = 1
	}
	return &UserQuota{
		running: make(map[string]int),
		limit:   limit,
	}
}

// TryAcquire attempts to acquire a slot for the user. Returns true if acquired.
func (q *UserQuota) TryAcquire(userID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.running[userID] >= q.limit {
		return false
	}
	q.running[userID]++
	return true
}

// Release releases a slot for the user.
func (q *UserQuota) Release(userID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.running[userID] > 0 {
		q.running[userID]--
	}
	if q.running[userID] == 0 {
		delete(q.running, userID)
	}
}

// Running returns the current running task count for the user.
func (q *UserQuota) Running(userID string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.running[userID]
}
