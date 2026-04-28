// Package ratelimit oferă un token-bucket simplu, in-memory, sigur pentru goroutine.
//
// Atenție: e in-process — la mai multe instanțe (Faza 2.3 cu shared subscription) un
// device poate alterna între workers și efectivul rate cumulat e mai mare decât configurat.
// Pentru rate limiting cross-instance, vezi Faza 2.4 (Redis-backed).
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter aplică două nivele paralele: per-device și per-tenant. Allow() returnează false
// dacă oricare dintre cele două buckets nu mai are token disponibil.
type Limiter struct {
	mu             sync.Mutex
	deviceBuckets  map[string]*bucket
	tenantBuckets  map[string]*bucket
	deviceRate     float64 // tokens/sec
	deviceCapacity float64 // max burst
	tenantRate     float64
	tenantCapacity float64
}

func New(deviceRate, deviceCapacity, tenantRate, tenantCapacity float64) *Limiter {
	return &Limiter{
		deviceBuckets:  make(map[string]*bucket),
		tenantBuckets:  make(map[string]*bucket),
		deviceRate:     deviceRate,
		deviceCapacity: deviceCapacity,
		tenantRate:     tenantRate,
		tenantCapacity: tenantCapacity,
	}
}

// Allow consumă câte un token din bucket-ul device-ului ȘI din cel al tenantului.
// Dacă vreunul nu are token disponibil, returnează false fără să consume nimic.
func (l *Limiter) Allow(deviceID, tenantID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	db := l.getBucket(l.deviceBuckets, deviceID, l.deviceCapacity, now)
	tb := l.getBucket(l.tenantBuckets, tenantID, l.tenantCapacity, now)

	l.refill(db, l.deviceRate, l.deviceCapacity, now)
	l.refill(tb, l.tenantRate, l.tenantCapacity, now)

	if db.tokens < 1 || tb.tokens < 1 {
		return false
	}
	db.tokens -= 1
	tb.tokens -= 1
	return true
}

func (l *Limiter) getBucket(m map[string]*bucket, key string, cap float64, now time.Time) *bucket {
	b, ok := m[key]
	if !ok {
		b = &bucket{tokens: cap, last: now}
		m[key] = b
	}
	return b
}

func (l *Limiter) refill(b *bucket, rate, cap float64, now time.Time) {
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rate
		if b.tokens > cap {
			b.tokens = cap
		}
		b.last = now
	}
}
