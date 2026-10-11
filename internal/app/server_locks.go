package app

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type chatRequestKey struct {
	workspaceID string
	requestID   string
}

type chatRequestLockManager struct {
	mu       sync.Mutex
	requests map[chatRequestKey]chan struct{}
	poolKey  string
}

type chatRequestLockPoolEntry struct {
	manager *chatRequestLockManager
	refs    int
}

var chatRequestLockPool = struct {
	sync.Mutex
	entries map[string]*chatRequestLockPoolEntry
}{entries: make(map[string]*chatRequestLockPoolEntry)}

func newChatRequestLockManager() *chatRequestLockManager {
	return &chatRequestLockManager{requests: make(map[chatRequestKey]chan struct{})}
}

func pooledChatRequestLockManager(databasePath string) (*chatRequestLockManager, func()) {
	if databasePath == ":memory:" {
		return newChatRequestLockManager(), func() {}
	}
	key := databasePath
	if !strings.HasPrefix(databasePath, "file:") {
		key = filepath.Clean(databasePath)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
	}
	chatRequestLockPool.Lock()
	entry := chatRequestLockPool.entries[key]
	if entry == nil {
		manager := newChatRequestLockManager()
		manager.poolKey = key
		entry = &chatRequestLockPoolEntry{manager: manager}
		chatRequestLockPool.entries[key] = entry
	}
	entry.refs++
	manager := entry.manager
	chatRequestLockPool.Unlock()
	var once sync.Once
	return manager, func() {
		once.Do(func() { releasePooledChatRequestLockManager(key, manager) })
	}
}

func releasePooledChatRequestLockManager(key string, manager *chatRequestLockManager) {
	if key == "" {
		return
	}
	chatRequestLockPool.Lock()
	if entry := chatRequestLockPool.entries[key]; entry != nil && entry.manager == manager {
		entry.refs--
		if entry.refs <= 0 {
			delete(chatRequestLockPool.entries, key)
		}
	}
	chatRequestLockPool.Unlock()
}

func (m *chatRequestLockManager) retain() func() {
	if m == nil || m.poolKey == "" {
		return func() {}
	}
	chatRequestLockPool.Lock()
	if entry := chatRequestLockPool.entries[m.poolKey]; entry != nil && entry.manager == m {
		entry.refs++
	}
	chatRequestLockPool.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() { releasePooledChatRequestLockManager(m.poolKey, m) })
	}
}

func (s *Server) acquireChatRequest(ctx context.Context, workspaceID, requestID string) (func(), error) {
	key := chatRequestKey{workspaceID: workspaceID, requestID: requestID}
	locks := s.chatRequestLocks
	if locks == nil {
		locks = newChatRequestLockManager()
		s.chatRequestLocks = locks
	}
	releasePoolRef := locks.retain()
	for {
		if err := ctx.Err(); err != nil {
			releasePoolRef()
			return nil, err
		}
		locks.mu.Lock()
		if locks.requests == nil {
			locks.requests = make(map[chatRequestKey]chan struct{})
		}
		if inFlight, exists := locks.requests[key]; exists {
			locks.mu.Unlock()
			select {
			case <-inFlight:
				continue
			case <-ctx.Done():
				releasePoolRef()
				return nil, ctx.Err()
			}
		}
		done := make(chan struct{})
		locks.requests[key] = done
		locks.mu.Unlock()
		var once sync.Once
		return func() {
			once.Do(func() {
				locks.mu.Lock()
				if locks.requests[key] == done {
					delete(locks.requests, key)
				}
				close(done)
				locks.mu.Unlock()
				releasePoolRef()
			})
		}, nil
	}
}
