// cmd/downlink-worker — Faza 3.3: worker downlink comenzi device.
//
// Funcționare:
//  1. BRPOP blocking pe lista Redis "cmd:queue" (scrisă de Django la POST /api/devices/{id}/commands/)
//  2. Publică comanda pe MQTT: tenants/{tenantID}/devices/{serial}/down/cmd (QoS 1)
//  3. Actualizează status → "sent" via PATCH /api/devices/commands/{id}/ack/
//
// La startup: Login Django → Login MQTT. Graceful shutdown pe SIGTERM/SIGINT.
package main

import (
	"context"
	"encoding/json"
	"errors"
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
	"go-iot-platform/internal/logging"
)

type CommandMessage struct {
	CommandID int64                  `json:"command_id"`
	TenantID  int64                  `json:"tenant_id"`
	Serial    string                 `json:"serial"`
	Action    string                 `json:"action"`
	Payload   map[string]interface{} `json:"payload"`
}

func main() {
	_ = godotenv.Load()

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("🚀 downlink-worker starting…")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := django.Login(os.Getenv("DJANGO_SERVICE_USER"), os.Getenv("DJANGO_SERVICE_PASS")); err != nil {
		log.Fatalf("Django login failed: %v", err)
	}
	log.Println("✅ Django login OK")

	mqttBroker := os.Getenv("MQTT_BROKER")
	if mqttBroker == "" {
		log.Fatal("MQTT_BROKER not set")
	}

	clientID := fmt.Sprintf("downlink-worker-%d", time.Now().UnixNano())
	mqttOpts := mqtt.NewClientOptions()
	mqttOpts.AddBroker(mqttBroker)
	mqttOpts.SetUsername(os.Getenv("MQTT_USER"))
	mqttOpts.SetPassword(os.Getenv("MQTT_PASS"))
	mqttOpts.SetClientID(clientID)
	mqttOpts.SetCleanSession(true)

	pubClient := mqtt.NewClient(mqttOpts)
	if token := pubClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("MQTT connect failed: %v", token.Error())
	}
	log.Println("✅ MQTT connected")
	defer pubClient.Disconnect(500)

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		log.Fatal("REDIS_ADDR not set — downlink-worker requires Redis")
	}
	dbNum := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			dbNum = n
		}
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       dbNum,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Redis ping failed: %v", err)
	}
	log.Println("✅ Redis connected, waiting for commands on cmd:queue…")

	for {
		select {
		case <-ctx.Done():
			log.Println("🛑 downlink-worker shutting down")
			return
		default:
		}

		// BRPOP cu timeout 2s permitem verificarea ctx.Done() periodic.
		result, err := rdb.BRPop(ctx, 2*time.Second, "cmd:queue").Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // timeout normal
			}
			if errors.Is(err, context.Canceled) {
				log.Println("🛑 Context cancelled, exiting")
				return
			}
			logging.Warn("BRPop error", logging.Fields{"error": err.Error()})
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// result = ["cmd:queue", "<json>"]
		if len(result) < 2 {
			continue
		}

		var msg CommandMessage
		if err := json.Unmarshal([]byte(result[1]), &msg); err != nil {
			logging.Warn("command parse failed", logging.Fields{"raw": result[1], "error": err.Error()})
			continue
		}

		logging.Info("dispatching command", logging.Fields{
			"command_id": msg.CommandID,
			"serial":     msg.Serial,
			"action":     msg.Action,
		})

		mqttPayload, _ := json.Marshal(map[string]interface{}{
			"command_id": msg.CommandID,
			"action":     msg.Action,
			"payload":    msg.Payload,
		})
		topic := fmt.Sprintf("tenants/%d/devices/%s/down/cmd", msg.TenantID, msg.Serial)

		token := pubClient.Publish(topic, 1, false, mqttPayload)
		token.Wait()
		if token.Error() != nil {
			logging.Warn("MQTT publish failed", logging.Fields{
				"command_id": msg.CommandID,
				"topic":      topic,
				"error":      token.Error().Error(),
			})
			continue
		}

		if err := django.AckCommand(msg.CommandID, "sent", nil); err != nil {
			logging.Warn("AckCommand (sent) failed", logging.Fields{
				"command_id": msg.CommandID,
				"error":      err.Error(),
			})
		}
	}
}
