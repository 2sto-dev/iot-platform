// cmd/mqtt-bridge translatează topicuri vendor-shaped (legacy) în schema tenant-aware.
//
// Connectează la EMQX (același broker ca ingest-ul), se abonează la pattern-uri vendor
// (NU shared — o singură instanță de bridge pentru a evita publish-uri duplicate),
// pentru fiecare mesaj face lookup serial→tenant via Redis cache, și republică pe
// `tenants/{tid}/devices/{did}/up/{stream}` pe ACELAȘI broker.
//
// Mesajul original rămâne pe topicul legacy. Ingest-ul (cmd/main.go) trebuie să nu se
// mai aboneze la pattern-uri legacy când bridge-ul e activ — vezi env ENABLE_LEGACY_SUBS=false.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"go-iot-platform/internal/bridge"
	"go-iot-platform/internal/cache"
	"go-iot-platform/internal/django"
	"go-iot-platform/internal/logging"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("🌉 mqtt-bridge starting")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := django.Login(os.Getenv("DJANGO_SERVICE_USER"), os.Getenv("DJANGO_SERVICE_PASS")); err != nil {
		log.Fatalf("Eroare login Django: %v", err)
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR e obligatoriu pentru bridge (lookup serial→tenant)")
	}
	dbNum := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			dbNum = n
		}
	}
	deviceCache, err := cache.New(ctx, cache.Config{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       dbNum,
	})
	if err != nil {
		log.Fatalf("Redis cache init: %v", err)
	}
	defer deviceCache.Close()
	if err := deviceCache.Warm(ctx); err != nil {
		log.Printf("⚠️ cache warm: %v", err)
	}
	go func() {
		for err := range deviceCache.SubscribeInvalidations(ctx) {
			log.Printf("⚠️ cache invalidation error: %v", err)
		}
	}()

	mqttBroker := os.Getenv("MQTT_BROKER")
	if mqttBroker == "" {
		log.Fatal("MQTT_BROKER nu e setat")
	}
	clientID := os.Getenv("MQTT_BRIDGE_CLIENT_ID")
	if clientID == "" {
		clientID = "mqtt-bridge-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttBroker)
	opts.SetUsername(os.Getenv("MQTT_USER"))
	opts.SetPassword(os.Getenv("MQTT_PASS"))
	opts.SetClientID(clientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)

	client := mqtt.NewClient(opts)
	if t := client.Connect(); t.Wait() && t.Error() != nil {
		log.Fatalf("MQTT connect: %v", t.Error())
	}
	log.Printf("✅ MQTT connected as %s", clientID)

	handler := func(c mqtt.Client, msg mqtt.Message) {
		go translateAndPublish(c, deviceCache, msg)
	}

	for _, p := range bridge.LegacyPatterns() {
		if t := client.Subscribe(p, 0, handler); t.Wait() && t.Error() != nil {
			log.Printf("❌ subscribe %s: %v", p, t.Error())
		} else {
			log.Printf("✅ subscribed: %s", p)
		}
	}

	<-ctx.Done()
	log.Println("🛑 mqtt-bridge shutting down")
	client.Disconnect(2000)
}

func translateAndPublish(c mqtt.Client, deviceCache *cache.Cache, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	// Lookup device → tenant via cache (Redis primary, Django fallback).
	parts := splitFirst2(topic)
	if parts.serial == "" {
		logging.Drop("bridge: topic too short", logging.Fields{"topic": topic})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tenantID, ok := deviceCache.GetDeviceTenant(ctx, parts.serial)
	if !ok || tenantID <= 0 {
		logging.Warn("bridge: device unknown — skip translation", logging.Fields{
			"topic": topic, "serial": parts.serial,
		})
		return
	}

	res := bridge.Translate(topic, tenantID)
	if res.Skip {
		logging.Warn("bridge: translation skipped", logging.Fields{
			"topic": topic, "reason": res.Reason,
		})
		return
	}

	t := c.Publish(res.NewTopic, 0, false, payload)
	if !t.WaitTimeout(2 * time.Second) {
		logging.Error("bridge: publish timeout", logging.Fields{
			"new_topic": res.NewTopic, "device_id": res.DeviceID,
		})
		return
	}
	if err := t.Error(); err != nil {
		logging.Error("bridge: publish error", logging.Fields{
			"new_topic": res.NewTopic, "error": err.Error(),
		})
		return
	}
	logging.Info("bridge: translated", logging.Fields{
		"from": topic, "to": res.NewTopic, "tenant_id": tenantID, "device_id": res.DeviceID,
	})
}

type topicParts struct{ vendor, serial string }

func splitFirst2(topic string) topicParts {
	out := topicParts{}
	idx := indexOf(topic, '/')
	if idx < 0 {
		return out
	}
	out.vendor = topic[:idx]
	rest := topic[idx+1:]
	idx2 := indexOf(rest, '/')
	if idx2 < 0 {
		out.serial = rest
		return out
	}
	out.serial = rest[:idx2]
	return out
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
