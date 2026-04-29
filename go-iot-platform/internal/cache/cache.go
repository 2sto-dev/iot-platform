// Package cache oferă lookup device→tenant cu Redis ca primary store și fallback HTTP la Django.
//
// Comportament:
//   - GetDeviceTenant(serial) caută în Redis: dacă hit, returnează direct (ms latency)
//   - Miss: cere lista completă de la Django, repopulează Redis cu TTL_LONG, returnează
//   - Subscribe la canalul "device-cache-invalidate" pentru invalidări push de la Django
//
// Faza 2.4 înlocuiește apelul GetAllDevices() per-message din cmd/main.go.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"go-iot-platform/internal/django"
)

const (
	keyPrefix         = "device:"
	invalidateChannel = "device-cache-invalidate"
	defaultTTL        = 10 * time.Minute
)

type Entry struct {
	Serial   string `json:"serial"`
	TenantID int64  `json:"tenant_id"`
}

type Cache struct {
	rdb       *redis.Client
	ttl       time.Duration
	mu        sync.RWMutex
	negative  map[string]time.Time // serial → expires_at pentru "device necunoscut"
	negTTL    time.Duration
	hits      uint64
	misses    uint64
	statsMu   sync.Mutex
}

type Config struct {
	Addr     string
	Password string
	DB       int
	TTL      time.Duration // pozitiv: Redis key expiry; default 10min
	NegTTL   time.Duration // negative cache pentru misses; default 30s
}

func New(ctx context.Context, cfg Config) (*Cache, error) {
	if cfg.TTL == 0 {
		cfg.TTL = defaultTTL
	}
	if cfg.NegTTL == 0 {
		cfg.NegTTL = 30 * time.Second
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &Cache{
		rdb:      rdb,
		ttl:      cfg.TTL,
		negative: make(map[string]time.Time),
		negTTL:   cfg.NegTTL,
	}, nil
}

// GetDeviceTenant returnează tenant_id pentru un device serial.
// Returnează (tenantID, true) la hit / device cunoscut.
// Returnează (0, false) când device-ul nu există (cu negative cache pentru a evita Django thrashing).
func (c *Cache) GetDeviceTenant(ctx context.Context, serial string) (int64, bool) {
	// 1. Negative cache check
	c.mu.RLock()
	if exp, ok := c.negative[serial]; ok && time.Now().Before(exp) {
		c.mu.RUnlock()
		c.bumpStat(false)
		return 0, false
	}
	c.mu.RUnlock()

	// 2. Redis lookup
	val, err := c.rdb.Get(ctx, keyPrefix+serial).Result()
	if err == nil {
		var e Entry
		if jerr := json.Unmarshal([]byte(val), &e); jerr == nil {
			c.bumpStat(true)
			return e.TenantID, true
		}
	}

	// 3. Miss → fallback la Django (refresh complet, populează tot)
	if err := c.refresh(ctx); err != nil {
		c.bumpStat(false)
		return 0, false
	}

	// 4. Re-check Redis după refresh
	val, err = c.rdb.Get(ctx, keyPrefix+serial).Result()
	if err == nil {
		var e Entry
		if jerr := json.Unmarshal([]byte(val), &e); jerr == nil {
			c.bumpStat(true)
			return e.TenantID, true
		}
	}

	// 5. Tot miss → negative cache
	c.mu.Lock()
	c.negative[serial] = time.Now().Add(c.negTTL)
	c.mu.Unlock()
	c.bumpStat(false)
	return 0, false
}

// refresh repopulează cache-ul cu lista completă de device-uri din Django.
// Folosit la miss și ca warm-up la startup.
func (c *Cache) refresh(ctx context.Context) error {
	devices, err := django.GetAllDevices()
	if err != nil {
		return fmt.Errorf("django GetAllDevices: %w", err)
	}
	pipe := c.rdb.Pipeline()
	for _, d := range devices {
		entry := Entry{Serial: d.Serial, TenantID: d.TenantID}
		b, _ := json.Marshal(entry)
		pipe.Set(ctx, keyPrefix+d.Serial, string(b), c.ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline: %w", err)
	}
	// Curăță negative cache la refresh — un device adăugat în Django să fie găsit imediat
	c.mu.Lock()
	c.negative = make(map[string]time.Time)
	c.mu.Unlock()
	return nil
}

// Warm pre-populează cache-ul la startup.
func (c *Cache) Warm(ctx context.Context) error {
	return c.refresh(ctx)
}

// Invalidate șterge un device din cache (chemat de subscriber-ul pub/sub).
func (c *Cache) Invalidate(ctx context.Context, serial string) error {
	c.mu.Lock()
	delete(c.negative, serial)
	c.mu.Unlock()
	return c.rdb.Del(ctx, keyPrefix+serial).Err()
}

// SubscribeInvalidations ascultă canalul Redis pub/sub și invalidează entry-urile.
// Mesajele trimise de Django (clients/signals.py) au format: {"serial": "..."}
func (c *Cache) SubscribeInvalidations(ctx context.Context) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		sub := c.rdb.Subscribe(ctx, invalidateChannel)
		defer sub.Close()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var payload struct {
					Serial string `json:"serial"`
				}
				if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
					errCh <- fmt.Errorf("invalidation parse: %w", err)
					continue
				}
				if payload.Serial == "" {
					// "" = invalidează tot
					_ = c.refresh(ctx)
					continue
				}
				_ = c.Invalidate(ctx, payload.Serial)
			}
		}
	}()
	return errCh
}

// Stats returnează (hits, misses) pentru observabilitate.
func (c *Cache) Stats() (uint64, uint64) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	return c.hits, c.misses
}

func (c *Cache) bumpStat(hit bool) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	if hit {
		c.hits++
	} else {
		c.misses++
	}
}

// Close închide conexiunea Redis.
func (c *Cache) Close() error {
	return c.rdb.Close()
}

// ParseTenantTag transformă tenant_id int64 într-un tag string (folosit la Influx).
func ParseTenantTag(id int64) string {
	if id <= 0 {
		return "unassigned"
	}
	return strconv.FormatInt(id, 10)
}
