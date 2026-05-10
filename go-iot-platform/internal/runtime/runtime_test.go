package runtime

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// OnTelemetry + Get
// ============================================================================

func TestOnTelemetryCreates(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("dev1", 1, []string{"relay"}, "state", "nousat", map[string]any{"v": 1}, time.Minute)

	got, ok := m.Get("dev1")
	if !ok {
		t.Fatal("device not found after OnTelemetry")
	}
	if got.DeviceID != "dev1" {
		t.Errorf("device_id = %q, want dev1", got.DeviceID)
	}
	if got.TenantID != 1 {
		t.Errorf("tenant_id = %d, want 1", got.TenantID)
	}
	if !got.Online {
		t.Error("expected online=true")
	}
	if got.LastStream != "state" {
		t.Errorf("last_stream = %q", got.LastStream)
	}
	if got.LastFields["v"] != 1 {
		t.Errorf("last_fields not propagated")
	}
}

func TestOnTelemetryUpdates(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("dev1", 1, []string{"relay"}, "state", "nousat", map[string]any{"v": 1}, time.Minute)
	first, _ := m.Get("dev1")
	firstSeen := first.LastSeen

	time.Sleep(10 * time.Millisecond)
	m.OnTelemetry("dev1", 1, []string{"relay"}, "sensor", "nousat", map[string]any{"v": 2}, time.Minute)

	updated, _ := m.Get("dev1")
	if !updated.LastSeen.After(firstSeen) {
		t.Error("last_seen should advance")
	}
	if updated.LastStream != "sensor" {
		t.Errorf("last_stream not updated: %s", updated.LastStream)
	}
	if updated.LastFields["v"] != 2 {
		t.Errorf("last_fields not updated")
	}
}

func TestGetNonExistent(t *testing.T) {
	m := New(nil)
	_, ok := m.Get("ghost")
	if ok {
		t.Error("expected (nil, false) for non-existent device")
	}
}

func TestDefaultOfflineAfterApplied(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("dev1", 1, nil, "x", "y", nil, 0) // 0 → default
	got, _ := m.Get("dev1")
	if got.OfflineAfter != DefaultOfflineAfter {
		t.Errorf("OfflineAfter = %v, want %v", got.OfflineAfter, DefaultOfflineAfter)
	}
}

// ============================================================================
// ByTenant + ByCapability
// ============================================================================

func TestByTenant(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("d1", 1, []string{"relay"}, "state", "nousat", nil, time.Minute)
	m.OnTelemetry("d2", 1, []string{"relay"}, "state", "nousat", nil, time.Minute)
	m.OnTelemetry("d3", 2, []string{"inverter"}, "telemetry", "sun2000", nil, time.Minute)

	t1 := m.ByTenant(1)
	if len(t1) != 2 {
		t.Errorf("tenant 1: expected 2, got %d", len(t1))
	}
	t2 := m.ByTenant(2)
	if len(t2) != 1 {
		t.Errorf("tenant 2: expected 1, got %d", len(t2))
	}
	t3 := m.ByTenant(99)
	if len(t3) != 0 {
		t.Errorf("tenant 99: expected 0, got %d", len(t3))
	}
}

func TestByCapability(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("plug1", 1, []string{"smart_plug", "relay", "power_meter"}, "state", "nousat", nil, time.Minute)
	m.OnTelemetry("inv1", 1, []string{"hybrid_inverter", "inverter", "battery"}, "telemetry", "sun2000", nil, time.Minute)
	m.OnTelemetry("plug2", 1, []string{"smart_plug", "relay", "power_meter"}, "state", "nousat", nil, time.Minute)

	relays := m.ByCapability(1, "relay")
	if len(relays) != 2 {
		t.Errorf("relay: expected 2, got %d", len(relays))
	}
	inverters := m.ByCapability(1, "inverter")
	if len(inverters) != 1 {
		t.Errorf("inverter: expected 1, got %d", len(inverters))
	}
	// Cross-tenant isolation
	other := m.ByCapability(2, "relay")
	if len(other) != 0 {
		t.Errorf("tenant 2: expected 0, got %d", len(other))
	}
}

// ============================================================================
// MarkOffline
// ============================================================================

func TestMarkOffline(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("dev1", 1, nil, "x", "y", nil, time.Minute)

	if changed := m.MarkOffline("dev1"); !changed {
		t.Error("expected MarkOffline to return true (online → offline)")
	}
	got, _ := m.Get("dev1")
	if got.Online {
		t.Error("expected offline after MarkOffline")
	}

	// Idempotent: a doua oara return false
	if changed := m.MarkOffline("dev1"); changed {
		t.Error("expected MarkOffline idempotent → false")
	}
}

func TestMarkOfflineUnknown(t *testing.T) {
	m := New(nil)
	if changed := m.MarkOffline("ghost"); changed {
		t.Error("MarkOffline on unknown device should return false")
	}
}

// ============================================================================
// Offline detector
// ============================================================================

func TestDetectOfflineOnce(t *testing.T) {
	m := New(nil)
	// Device with very short OfflineAfter
	m.OnTelemetry("dev1", 1, nil, "x", "y", nil, 50*time.Millisecond)
	// Device with long OfflineAfter (won't expire)
	m.OnTelemetry("dev2", 1, nil, "x", "y", nil, 1*time.Hour)

	// Wait past dev1's threshold
	time.Sleep(100 * time.Millisecond)
	m.detectOfflineOnce()

	d1, _ := m.Get("dev1")
	if d1.Online {
		t.Error("dev1 should be offline after detector")
	}
	d2, _ := m.Get("dev2")
	if !d2.Online {
		t.Error("dev2 should still be online")
	}
}

func TestStartOfflineDetector(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("dev1", 1, nil, "x", "y", nil, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	m.StartOfflineDetector(ctx, 30*time.Millisecond)

	time.Sleep(120 * time.Millisecond)

	d1, _ := m.Get("dev1")
	if d1.Online {
		t.Error("dev1 should be offline after detector ticks")
	}
}

// ============================================================================
// Concurrent access
// ============================================================================

func TestConcurrentUpdates(t *testing.T) {
	m := New(nil)
	const N = 1000
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			devID := "dev"
			m.OnTelemetry(devID, 1, nil, "x", "y", map[string]any{"i": id}, time.Minute)
		}(i)
	}
	wg.Wait()

	got, ok := m.Get("dev")
	if !ok {
		t.Fatal("device should exist after concurrent updates")
	}
	if !got.Online {
		t.Error("expected online")
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	m := New(nil)
	m.OnTelemetry("dev1", 1, nil, "x", "y", nil, time.Minute)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				m.OnTelemetry("dev1", 1, nil, "x", "y", nil, time.Minute)
			}
		}
	}()

	// Multiple reader goroutines
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = m.Get("dev1")
					_ = m.ByTenant(1)
				}
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
	// Test trece dacă nu e race detected (`go test -race`).
}

// ============================================================================
// HasCapability
// ============================================================================

func TestDeviceRuntimeHasCapability(t *testing.T) {
	d := &DeviceRuntime{Capabilities: []string{"smart_plug", "relay", "power_meter"}}
	if !d.HasCapability("relay") {
		t.Error("relay should be present")
	}
	if d.HasCapability("inverter") {
		t.Error("inverter should not be present")
	}
}

func TestAgeSinceLastSeen(t *testing.T) {
	d := &DeviceRuntime{}
	if d.AgeSinceLastSeen() >= 0 {
		t.Error("zero last_seen should give negative age")
	}
	d.LastSeen = time.Now().Add(-30 * time.Second)
	age := d.AgeSinceLastSeen()
	if age < 25*time.Second || age > 35*time.Second {
		t.Errorf("expected ~30s, got %v", age)
	}
}

// ============================================================================
// Count + SnapshotAll
// ============================================================================

func TestCountAndSnapshot(t *testing.T) {
	m := New(nil)
	if m.Count() != 0 {
		t.Errorf("empty count")
	}
	m.OnTelemetry("a", 1, nil, "x", "y", nil, time.Minute)
	m.OnTelemetry("b", 1, nil, "x", "y", nil, time.Minute)
	m.OnTelemetry("c", 2, nil, "x", "y", nil, time.Minute)

	if m.Count() != 3 {
		t.Errorf("count = %d", m.Count())
	}

	snap := m.SnapshotAll()
	if len(snap) != 3 {
		t.Errorf("snapshot len = %d", len(snap))
	}
}

// ============================================================================
// Benchmark — target < 5µs OnTelemetry
// ============================================================================

func BenchmarkOnTelemetry(b *testing.B) {
	m := New(nil)
	caps := []string{"smart_plug", "relay", "power_meter"}
	fields := map[string]any{"power": 100.0, "voltage": 230.0}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.OnTelemetry("dev", 1, caps, "sensor", "nousat", fields, time.Minute)
	}
}

func BenchmarkGet(b *testing.B) {
	m := New(nil)
	m.OnTelemetry("dev", 1, nil, "x", "y", nil, time.Minute)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = m.Get("dev")
	}
}

func BenchmarkByTenant(b *testing.B) {
	m := New(nil)
	for i := 0; i < 100; i++ {
		// Spread across 5 tenants
		m.OnTelemetry("dev"+string(rune('a'+i%26))+string(rune('a'+(i/26)%26)), int64(i%5+1),
			[]string{"relay"}, "state", "nousat", nil, time.Minute)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.ByTenant(1)
	}
}
