package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const cacheKeyPrefix = "rules:v1:"

// RuleCache fetches and caches rules per tenant in Redis.
// On cache miss it calls the Django internal API.
type RuleCache struct {
	rdb        *redis.Client
	djangoBase string
	svcUser    string
	svcPass    string
	httpClient *http.Client
}

func NewRuleCache(rdb *redis.Client, djangoBase, svcUser, svcPass string) *RuleCache {
	return &RuleCache{
		rdb:        rdb,
		djangoBase: djangoBase,
		svcUser:    svcUser,
		svcPass:    svcPass,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// GetRules returns enabled rules for a tenant.
// Tries Redis first; falls back to Django API on miss.
func (c *RuleCache) GetRules(ctx context.Context, tenantID int64) ([]Rule, error) {
	key := fmt.Sprintf("%s%d", cacheKeyPrefix, tenantID)

	if c.rdb != nil {
		data, err := c.rdb.Get(ctx, key).Bytes()
		if err == nil {
			var rules []Rule
			if json.Unmarshal(data, &rules) == nil {
				return rules, nil
			}
		}
	}

	rules, err := c.fetchFromDjango(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Populate cache with no TTL — invalidated by Django signal on rule change.
	if c.rdb != nil {
		if data, err := json.Marshal(rules); err == nil {
			c.rdb.Set(ctx, key, data, 0)
		}
	}
	return rules, nil
}

func (c *RuleCache) fetchFromDjango(ctx context.Context, tenantID int64) ([]Rule, error) {
	url := fmt.Sprintf("%s/api/internal/rules/?tenant_id=%d", c.djangoBase, tenantID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.svcUser, c.svcPass)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rules: django fetch: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("rules: django returned %d", resp.StatusCode)
	}
	var rules []Rule
	if err := json.Unmarshal(body, &rules); err != nil {
		return nil, fmt.Errorf("rules: parse: %w", err)
	}
	return rules, nil
}

// MatchesStream returns true if the rule's trigger_stream_pattern matches the stream.
// Pattern "*" matches any stream. Comma-separated list matches any element.
func MatchesStream(rule Rule, stream string) bool {
	pattern := strings.TrimSpace(rule.TriggerStreamPattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	for _, p := range strings.Split(pattern, ",") {
		if strings.TrimSpace(p) == stream {
			return true
		}
	}
	return false
}

// CheckAndSetCooldown returns true if the rule can fire (cooldown not active).
// On first call in the window, sets the Redis key so subsequent calls return false.
func CheckAndSetCooldown(ctx context.Context, rdb *redis.Client, ruleID int64, serial string, cooldownSec int) bool {
	if rdb == nil || cooldownSec <= 0 {
		return true
	}
	key := fmt.Sprintf("rule_cooldown:%d:%s", ruleID, serial)
	set, err := rdb.SetNX(ctx, key, "1", time.Duration(cooldownSec)*time.Second).Result()
	if err != nil {
		log.Printf("rules: cooldown redis error: %v", err)
		return true // fail open
	}
	return set
}

// GetPrevState returns the stored previous field values for a device (for "changed" op).
func GetPrevState(ctx context.Context, rdb *redis.Client, tenantID int64, serial string) map[string]interface{} {
	if rdb == nil {
		return nil
	}
	key := fmt.Sprintf("rule_prev:%d:%s", tenantID, serial)
	data, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil
	}
	var state map[string]interface{}
	json.Unmarshal(data, &state) //nolint:errcheck
	return state
}

// SetPrevState saves the current payload as previous state (TTL = 5 min).
func SetPrevState(ctx context.Context, rdb *redis.Client, tenantID int64, serial string, payload map[string]interface{}) {
	if rdb == nil {
		return
	}
	key := fmt.Sprintf("rule_prev:%d:%s", tenantID, serial)
	if data, err := json.Marshal(payload); err == nil {
		rdb.Set(ctx, key, data, 5*time.Minute) //nolint:errcheck
	}
}

// ParseTopic parses "tenants/{tid}/devices/{serial}/up/{stream}" into components.
func ParseTopic(topic string) (tenantID int64, serial, stream string, ok bool) {
	parts := strings.Split(topic, "/")
	if len(parts) < 6 || parts[0] != "tenants" || parts[2] != "devices" || parts[4] != "up" {
		return
	}
	tid, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}
	return tid, parts[3], strings.Join(parts[5:], "/"), true
}
