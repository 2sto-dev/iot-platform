// cmd/mqtt-bridge — Faza 2.2: MQTT bridge pentru device-uri legacy.
//
// Primește mesaje pe topic-uri vendor (shellies/+/#, tele/+/#, zigbee2mqtt/+)
// și le re-publică pe schema tenant-scoped (tenants/{tid}/devices/{serial}/up/{stream})
// după lookup serial→tenant via Redis cache / Django fallback.
//
// Deployment: rulează în paralel cu cmd/main (ingest worker); are clientID unic.
// Când bridge-ul e activ în prod, eliminați subscription-urile legacy din cmd/main.go.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"

	"go-iot-platform/internal/bridge"
	"go-iot-platform/internal/cache"
	"go-iot-platform/internal/django"
	"go-iot-platform/internal/logging"
)

func main() {
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := django.Login(os.Getenv("DJANGO_SERVICE_USER"), os.Getenv("DJANGO_SERVICE_PASS")); err != nil {
		log.Fatalf("bridge: django login: %v", err)
	}

	var deviceCache *cache.Cache
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		dbNum := 0
		if v := os.Getenv("REDIS_DB"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				dbNum = n
			}
		}
		c, err := cache.New(ctx, cache.Config{
			Addr:     addr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       dbNum,
		})
		if err != nil {
			log.Printf("bridge: redis unavailable (%v); fallback la Django per-message", err)
		} else {
			deviceCache = c
			defer deviceCache.Close()
			if err := deviceCache.Warm(ctx); err != nil {
				log.Printf("bridge: cache warm: %v", err)
			}
			go func() {
				for err := range deviceCache.SubscribeInvalidations(ctx) {
					log.Printf("bridge: cache invalidation error: %v", err)
				}
			}()
			log.Println("bridge: Redis cache warmed")
		}
	} else {
		log.Println("bridge: REDIS_ADDR not set; fallback Django per-message (slower)")
	}

	broker := os.Getenv("MQTT_BROKER")
	if broker == "" {
		log.Fatal("bridge: MQTT_BROKER not set")
	}
	clientID := os.Getenv("MQTT_BRIDGE_CLIENT_ID")
	if clientID == "" {
		clientID = fmt.Sprintf("mqtt-bridge-%d", time.Now().UnixNano())
	}

	pubClient := newMQTTClient(broker, clientID+"-pub",
		os.Getenv("MQTT_USER"), os.Getenv("MQTT_PASS"))
	if token := pubClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("bridge: pub connect: %v", token.Error())
	}
	defer pubClient.Disconnect(500)

	legacyTopics := []string{
		"$share/bridge/shellies/+/#",
		"$share/bridge/tele/+/#",
		"$share/bridge/zigbee2mqtt/+",
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID + "-sub")
	opts.SetUsername(os.Getenv("MQTT_USER"))
	opts.SetPassword(os.Getenv("MQTT_PASS"))
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)

	handler := makeHandler(ctx, deviceCache, pubClient)
	opts.OnConnect = func(c mqtt.Client) {
		for _, topic := range legacyTopics {
			if tok := c.Subscribe(topic, 0, handler); tok.Wait() && tok.Error() != nil {
				log.Printf("bridge: subscribe %s: %v", topic, tok.Error())
			} else {
				log.Printf("bridge: subscribed %s", topic)
			}
		}
	}

	sub := mqtt.NewClient(opts)
	if tok := sub.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatalf("bridge: sub connect: %v", tok.Error())
	}
	defer sub.Disconnect(500)

	log.Println("bridge: running — Ctrl-C to stop")
	<-ctx.Done()
	log.Println("bridge: shutting down")
}

func newMQTTClient(broker, clientID, user, pass string) mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.SetUsername(user)
	opts.SetPassword(pass)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	return mqtt.NewClient(opts)
}

func makeHandler(ctx context.Context, c *cache.Cache, pub mqtt.Client) mqtt.MessageHandler {
	return func(_ mqtt.Client, msg mqtt.Message) {
		topic := msg.Topic()
		serial, stream, ok := bridge.ParseLegacy(topic)
		if !ok {
			logging.Warn("bridge: unrecognized legacy topic", logging.Fields{"topic": topic})
			return
		}

		var tenantID int64
		if c != nil {
			var found bool
			tenantID, found = c.GetDeviceTenant(ctx, serial)
			if !found {
				logging.Warn("bridge: unknown serial, dropping", logging.Fields{"serial": serial, "topic": topic})
				return
			}
		} else {
			// Fallback: cere tenant din Django (costisitor, doar când Redis nu e disponibil)
			devs, err := django.GetAllDevices()
			if err != nil {
				logging.Error("bridge: django fallback failed", logging.Fields{"error": err.Error()})
				return
			}
			for _, d := range devs {
				if d.Serial == serial {
					tenantID = d.TenantID
					break
				}
			}
			if tenantID == 0 {
				logging.Warn("bridge: serial not found in Django", logging.Fields{"serial": serial})
				return
			}
		}

		newTopic := bridge.NewTopic(tenantID, serial, stream)
		tok := pub.Publish(newTopic, msg.Qos(), false, msg.Payload())
		if tok.Wait() && tok.Error() != nil {
			logging.Error("bridge: publish failed", logging.Fields{
				"topic": newTopic,
				"error": tok.Error().Error(),
			})
			return
		}
		logging.Info("bridge: translated", logging.Fields{
			"from": topic,
			"to":   newTopic,
		})
	}
}
