package task

import "testing"

func TestUserQuotaEnforcesLimit(t *testing.T) {
	quota := NewUserQuota(2)
	if !quota.TryAcquire("alice") {
		t.Fatal("first acquire should succeed")
	}
	if quota.Running("alice") != 1 {
		t.Fatalf("alice running count = %d, want 1", quota.Running("alice"))
	}
	if !quota.TryAcquire("alice") {
		t.Fatal("second acquire should succeed")
	}
	if quota.Running("alice") != 2 {
		t.Fatalf("alice running count = %d, want 2", quota.Running("alice"))
	}
	if quota.TryAcquire("alice") {
		t.Fatal("third acquire should fail (limit reached)")
	}
	if quota.Running("alice") != 2 {
		t.Fatalf("alice running count = %d, want 2 (unchanged)", quota.Running("alice"))
	}
	quota.Release("alice")
	if quota.Running("alice") != 1 {
		t.Fatalf("alice running count = %d, want 1", quota.Running("alice"))
	}
	if !quota.TryAcquire("alice") {
		t.Fatal("acquire after release should succeed")
	}
}

func TestUserQuotaIsolatesUsers(t *testing.T) {
	quota := NewUserQuota(1)
	if !quota.TryAcquire("alice") {
		t.Fatal("alice acquire should succeed")
	}
	if !quota.TryAcquire("bob") {
		t.Fatal("bob acquire should succeed (independent of alice)")
	}
	if quota.TryAcquire("alice") {
		t.Fatal("alice second acquire should fail")
	}
	if quota.TryAcquire("bob") {
		t.Fatal("bob second acquire should fail")
	}
	quota.Release("alice")
	if !quota.TryAcquire("alice") {
		t.Fatal("alice acquire after release should succeed")
	}
	if quota.Running("bob") != 1 {
		t.Fatal("bob should still have 1 running (unaffected by alice)")
	}
}

func TestUserQuotaCleansUpZero(t *testing.T) {
	quota := NewUserQuota(2)
	quota.TryAcquire("alice")
	quota.Release("alice")
	quota.mu.Lock()
	if _, exists := quota.running["alice"]; exists {
		t.Fatal("alice entry should be deleted after count reaches zero")
	}
	quota.mu.Unlock()
}
