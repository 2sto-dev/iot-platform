// cmd/rule-engine — Faza 4.1: evaluator de reguli IoT în timp real.
//
// Subscrie la $share/rules/tenants/+/devices/+/up/#
// Pentru fiecare mesaj: încarcă regulile tenantului din Redis (sau Django),
// evaluează condițiile DSL, verifică cooldown, execută acțiunile.
//
// Acțiuni suportate:
//   - downlink:   publică MQTT pe tenants/{tid}/devices/{serial}/down/cmd
//   - notify:     POST la Django /api/internal/notifications/trigger/
//   - webhook:    HTTP call direct cu body template {{field}}
//   - set_shadow: PATCH Django shadow desired state
//
// Deployment: rulează în paralel cu go-iot-platform și downlink-worker.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"go-iot-platform/internal/django"
	"go-iot-platform/internal/rules"
)

func main() {
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ── Django auth ───────────────────────────────────────────────────────────
	if err := django.Login(os.Getenv("DJANGO_SERVICE_USER"), os.Getenv("DJANGO_SERVICE_PASS")); err != nil {
		log.Fatalf("rule-engine: django login: %v", err)
	}

	// ── Redis ─────────────────────────────────────────────────────────────────
	var rdb *redis.Client
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		dbNum := 0
		if v := os.Getenv("REDIS_DB"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				dbNum = n
			}
		}
		rdb = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       dbNum,
		})
		if _, err := rdb.Ping(ctx).Result(); err != nil {
			log.Printf("rule-engine: redis unavailable (%v); cooldown and cache disabled", err)
			rdb = nil
		} else {
			log.Println("rule-engine: Redis connected")
		}
	}

	djangoBase := os.Getenv("DJANGO_BASE_URL")
	// DJANGO_BASE_URL is typically "http://host:port/api" — strip /api suffix
	if len(djangoBase) > 4 && djangoBase[len(djangoBase)-4:] == "/api" {
		djangoBase = djangoBase[:len(djangoBase)-4]
	}
	svcUser := os.Getenv("DJANGO_SERVICE_USER")
	svcPass := os.Getenv("DJANGO_SERVICE_PASS")

	ruleCache := rules.NewRuleCache(rdb, djangoBase, svcUser, svcPass)

	// ── MQTT pub client (for downlink actions) ────────────────────────────────
	broker := os.Getenv("MQTT_BROKER")
	if broker == "" {
		log.Fatal("rule-engine: MQTT_BROKER not set")
	}
	clientID := fmt.Sprintf("rule-engine-pub-%d", time.Now().UnixNano())
	pubOpts := mqtt.NewClientOptions()
	pubOpts.AddBroker(broker)
	pubOpts.SetClientID(clientID)
	pubOpts.SetUsername(os.Getenv("MQTT_USER"))
	pubOpts.SetPassword(os.Getenv("MQTT_PASS"))
	pubOpts.SetAutoReconnect(true)
	pubOpts.SetMaxReconnectInterval(30 * time.Second)

	pubClient := mqtt.NewClient(pubOpts)
	if tok := pubClient.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatalf("rule-engine: pub connect: %v", tok.Error())
	}
	defer pubClient.Disconnect(500)

	executor := rules.NewExecutor(pubClient, djangoBase, svcUser, svcPass)

	// ── MQTT sub client ───────────────────────────────────────────────────────
	subClientID := fmt.Sprintf("rule-engine-sub-%d", time.Now().UnixNano())
	subOpts := mqtt.NewClientOptions()
	subOpts.AddBroker(broker)
	subOpts.SetClientID(subClientID)
	subOpts.SetUsername(os.Getenv("MQTT_USER"))
	subOpts.SetPassword(os.Getenv("MQTT_PASS"))
	subOpts.SetAutoReconnect(true)
	subOpts.SetMaxReconnectInterval(30 * time.Second)
	subOpts.OnConnect = func(c mqtt.Client) {
		topic := "$share/rules/tenants/+/devices/+/up/#"
		if tok := c.Subscribe(topic, 0, makeHandler(ctx, ruleCache, executor, rdb)); tok.Wait() && tok.Error() != nil {
			log.Printf("rule-engine: subscribe error: %v", tok.Error())
		} else {
			log.Printf("rule-engine: subscribed %s", topic)
		}
	}

	subClient := mqtt.NewClient(subOpts)
	if tok := subClient.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatalf("rule-engine: sub connect: %v", tok.Error())
	}
	defer subClient.Disconnect(500)

	log.Println("rule-engine: running — Ctrl-C to stop")
	<-ctx.Done()
	log.Println("rule-engine: shutting down")
}

func makeHandler(
	ctx context.Context,
	cache *rules.RuleCache,
	exec *rules.Executor,
	rdb *redis.Client,
) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		topic := msg.Topic()
		tenantID, serial, stream, ok := rules.ParseTopic(topic)
		if !ok {
			return
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
			log.Printf("rule-engine: invalid JSON on %s: %v", topic, err)
			return
		}

		ruleList, err := cache.GetRules(ctx, tenantID)
		if err != nil {
			log.Printf("rule-engine: load rules tenant %d: %v", tenantID, err)
			return
		}

		prevState := rules.GetPrevState(ctx, rdb, tenantID, serial)

		msgCtx := rules.MessageContext{
			TenantID: tenantID,
			Serial:   serial,
			Stream:   stream,
			Payload:  payload,
			RawTopic: topic,
		}

		for _, rule := range ruleList {
			if !rule.Enabled {
				continue
			}
			if !rules.MatchesStream(rule, stream) {
				continue
			}
			if !rules.Evaluate(rule.Conditions, payload, prevState) {
				continue
			}
			if !rules.CheckAndSetCooldown(ctx, rdb, rule.ID, serial, rule.CooldownSeconds) {
				logExecution(ctx, exec, rule, msgCtx, nil, rules.StatusCooldown, "")
				continue
			}

			results := exec.Execute(ctx, rule, msgCtx, 0)
			logExecution(ctx, exec, rule, msgCtx, results, rules.StatusTriggered, "")
			log.Printf("rule-engine: rule %q fired on %s/%s → %d actions", rule.Name, serial, stream, len(results))
		}

		rules.SetPrevState(ctx, rdb, tenantID, serial, payload)
	}
}

func logExecution(
	ctx context.Context,
	exec *rules.Executor,
	rule rules.Rule,
	msgCtx rules.MessageContext,
	actionResults []map[string]interface{},
	status rules.ExecStatus,
	errMsg string,
) {
	body := map[string]interface{}{
		"rule_id":            rule.ID,
		"rule_name":          rule.Name,
		"tenant_id":          msgCtx.TenantID,
		"device_serial":      msgCtx.Serial,
		"stream":             msgCtx.Stream,
		"conditions_snapshot": rule.Conditions,
		"actions_taken":      actionResults,
		"status":             string(status),
		"error_message":      errMsg,
	}
	if err := exec.LogExecution(ctx, body); err != nil {
		log.Printf("rule-engine: log failed: %v", err)
	}
}
