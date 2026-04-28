package ratelimit

import (
	"testing"
	"time"
)

func TestAllowWithinCapacity(t *testing.T) {
	// device: 10 msg/s burst 5; tenant: 100 msg/s burst 50
	l := New(10, 5, 100, 50)
	for i := 0; i < 5; i++ {
		if !l.Allow("dev1", "1") {
			t.Fatalf("expected allow at i=%d", i)
		}
	}
	if l.Allow("dev1", "1") {
		t.Fatalf("expected deny after burst exhausted")
	}
}

func TestRefill(t *testing.T) {
	l := New(10, 1, 100, 1)
	if !l.Allow("dev1", "1") {
		t.Fatalf("expected allow first")
	}
	if l.Allow("dev1", "1") {
		t.Fatalf("expected deny immediately after")
	}
	// Manually move "last" back to simulate elapsed time without sleeping.
	l.mu.Lock()
	l.deviceBuckets["dev1"].last = time.Now().Add(-200 * time.Millisecond)
	l.tenantBuckets["1"].last = time.Now().Add(-200 * time.Millisecond)
	l.mu.Unlock()
	if !l.Allow("dev1", "1") {
		t.Fatalf("expected allow after refill")
	}
}

func TestTenantLimitBlocksAcrossDevices(t *testing.T) {
	// tenant capacity 2, device capacity 100 → tenant should block 3rd device msg
	l := New(0, 100, 0, 2)
	if !l.Allow("d1", "1") {
		t.Fatalf("1st should allow")
	}
	if !l.Allow("d2", "1") {
		t.Fatalf("2nd should allow")
	}
	if l.Allow("d3", "1") {
		t.Fatalf("3rd should be blocked by tenant cap")
	}
}

func TestSeparateTenantsDontInterfere(t *testing.T) {
	l := New(0, 100, 0, 1)
	if !l.Allow("d1", "tenant-a") {
		t.Fatalf("tenant-a 1st allow")
	}
	if l.Allow("d1", "tenant-a") {
		t.Fatalf("tenant-a 2nd should deny")
	}
	if !l.Allow("d1", "tenant-b") {
		t.Fatalf("tenant-b 1st allow (different tenant)")
	}
}
