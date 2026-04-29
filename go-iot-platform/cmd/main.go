package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"go-iot-platform/internal/api"
	"go-iot-platform/internal/buffer"
	"go-iot-platform/internal/cache"
	"go-iot-platform/internal/django"
	"go-iot-platform/internal/influx"
	"go-iot-platform/internal/logging"
	"go-iot-platform/internal/ratelimit"
	"go-iot-platform/internal/topics"
)

var (
	titleCaser = cases.Title(language.Und)

	// Rate limit: 10 msg/s per device (burst 20), 200 msg/s per tenant (burst 400).
	limiter = ratelimit.New(10, 20, 200, 400)

	// Fallback fișier când Influx pică.
	influxBuffer *buffer.FileBuffer

	// Cache device→tenant cu Redis ca primary store + fallback Django (Faza 2.4).
	deviceCache *cache.Cache
)

func main() {
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		_ = os.Mkdir("logs", 0755)
	}
	f, _ := os.OpenFile("logs/go_meeter_runtime.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := django.Login(os.Getenv("DJANGO_SERVICE_USER"), os.Getenv("DJANGO_SERVICE_PASS")); err != nil {
		log.Fatalf("Eroare login Django: %v", err)
	}

	influxClient := influxdb2.NewClient(influx.URL, influx.Token)
	defer influxClient.Close()
	writeAPI := influxClient.WriteAPIBlocking(influx.Org, influx.Bucket)

	if buf, err := buffer.New("logs/influx_fallback.log"); err != nil {
		log.Printf("⚠️ Buffer fallback unavailable: %v (Influx errors will only be logged)", err)
	} else {
		influxBuffer = buf
		defer influxBuffer.Close()
	}

	// Faza 2.4: Redis cache pentru lookup device→tenant (înlocuiește GetAllDevices() per-message).
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		dbNum := 0
		if v := os.Getenv("REDIS_DB"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				dbNum = n
			}
		}
		c, err := cache.New(ctx, cache.Config{
			Addr:     redisAddr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       dbNum,
		})
		if err != nil {
			log.Printf("⚠️ Redis cache disabled (%v); fallback la Django per-message", err)
		} else {
			deviceCache = c
			defer deviceCache.Close()
			if err := deviceCache.Warm(ctx); err != nil {
				log.Printf("⚠️ cache warm failed: %v", err)
			} else {
				log.Println("✅ Redis cache device→tenant warmed")
			}
			go func() {
				for err := range deviceCache.SubscribeInvalidations(ctx) {
					log.Printf("⚠️ cache invalidation error: %v", err)
				}
			}()
		}
	} else {
		log.Println("⚠️ REDIS_ADDR not set; cache disabled, fallback la GetAllDevices() per-message")
	}

	go startMQTTSubscriber(ctx, writeAPI)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("⚠️ Request necunoscut: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: api.EnableCORS(http.StripPrefix("/go", mux)),
	}

	go func() {
		log.Println("✅ API Go disponibil pe http://localhost:8080/go/* prin Kong")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("🛑 Semnal primit, închidere graceful…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

func startMQTTSubscriber(ctx context.Context, writeAPI influxdb2api.WriteAPIBlocking) {
	mqttBroker := os.Getenv("MQTT_BROKER")
	mqttUsername := os.Getenv("MQTT_USER")
	mqttPassword := os.Getenv("MQTT_PASS")

	if mqttBroker == "" {
		log.Fatal("⚠️ MQTT_BROKER nu este setat în .env")
	}

	devices, err := django.GetAllDevices()
	if err != nil {
		log.Fatalf("Eroare la preluarea device-urilor din Django: %v", err)
	}

	var mqttTopics []string
	for _, d := range devices {
		mqttTopics = append(mqttTopics, d.Topics...)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttBroker)
	opts.SetUsername(mqttUsername)
	opts.SetPassword(mqttPassword)

	opts.OnConnect = func(c mqtt.Client) {
		for _, topic := range mqttTopics {
			if token := c.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
				go handleMessage(msg, writeAPI)
			}); token.Wait() && token.Error() != nil {
				log.Printf("Eroare la abonare topic %s: %v\n", topic, token.Error())
			} else {
				log.Printf("Abonat la topic (din Django): %s\n", topic)
			}
		}

		if token := c.Subscribe("#", 0, func(client mqtt.Client, msg mqtt.Message) {
			go handleMessage(msg, writeAPI)
		}); token.Wait() && token.Error() != nil {
			log.Printf("Eroare la abonare wildcard (#): %v\n", token.Error())
		} else {
			log.Println("Abonat la toate topicurile MQTT (#) pentru descoperire automată")
		}
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Eroare la conectarea MQTT: %v\n", token.Error())
	}

	<-ctx.Done()
	log.Println("🛑 MQTT: deconectare graceful…")
	client.Disconnect(250)
}

// --- Structuri JSON ---
type StateMessage struct {
	POWER string `json:"POWER"`
	RSSI  int    `json:"RSSI"`
}

type EnergyData struct {
	Total   float64 `json:"Total"`
	Power   float64 `json:"Power"`
	Voltage float64 `json:"Voltage"`
	Current float64 `json:"Current"`
}

type SensorMessage struct {
	Time   string     `json:"Time"`
	ENERGY EnergyData `json:"ENERGY"`
}

// writePoint scrie un punct în Influx cu fallback la fișier dacă scrierea eșuează.
// Loghează result-ul structurat (level=info la succes, level=error la eșec).
func writePoint(p *write.Point, writeAPI influxdb2api.WriteAPIBlocking, topic string, payload []byte, fields logging.Fields) {
	if err := writeAPI.WritePoint(context.Background(), p); err != nil {
		fields["error"] = err.Error()
		logging.Error("influx write failed", fields)
		if bufErr := influxBuffer.Append(topic, payload, err); bufErr != nil {
			logging.Error("buffer fallback failed", logging.Fields{"error": bufErr.Error()})
		}
		return
	}
	logging.Info("influx write ok", fields)
}

func handleMessage(msg mqtt.Message, writeAPI influxdb2api.WriteAPIBlocking) {
	topic := msg.Topic()
	payload := msg.Payload()

	logging.Info("mqtt message received", logging.Fields{"topic": topic, "size": len(payload)})

	// Parse topic
	parsed, err := topics.Parse(topic)
	if err != nil {
		logging.Drop("topic invalid", logging.Fields{"topic": topic, "error": err.Error()})
		return
	}

	var deviceID string
	if parsed.IsLegacy {
		deviceID = topics.LegacyDeviceID(topic)
	} else {
		deviceID = parsed.DeviceID
	}
	if deviceID == "" {
		logging.Drop("empty device_id", logging.Fields{"topic": topic})
		return
	}

	// Faza 2.4: lookup via Redis cache (cu fallback la Django dacă cache nu e configurat)
	tenantTag := "unassigned"
	var deviceTenantID int64
	found := false
	if deviceCache != nil {
		if tid, ok := deviceCache.GetDeviceTenant(context.Background(), deviceID); ok {
			found = true
			deviceTenantID = tid
			tenantTag = cache.ParseTenantTag(tid)
		}
	} else {
		// Fallback path când Redis nu e disponibil — comportamentul pre-2.4 (slow dar funcțional).
		devices, _ := django.GetAllDevices()
		for _, d := range devices {
			if d.Serial == deviceID {
				found = true
				deviceTenantID = d.TenantID
				if d.TenantID > 0 {
					tenantTag = strconv.FormatInt(d.TenantID, 10)
				}
				break
			}
		}
	}

	// #4 Validare device ↔ tenant pentru schema nouă
	if !parsed.IsLegacy {
		if !found {
			logging.Drop("unknown device on tenant-scoped topic", logging.Fields{
				"topic": topic, "device_id": deviceID, "tenant_id": parsed.TenantID,
			})
			return
		}
		if deviceTenantID != parsed.TenantID {
			logging.Drop("device-tenant mismatch", logging.Fields{
				"device_id":     deviceID,
				"topic_tenant":  parsed.TenantID,
				"device_tenant": deviceTenantID,
				"topic":         topic,
			})
			return
		}
	}

	// #10 Rate limit per device + per tenant
	if !limiter.Allow(deviceID, tenantTag) {
		logging.Drop("rate limited", logging.Fields{
			"device_id": deviceID, "tenant_id": tenantTag, "topic": topic,
		})
		return
	}

	if !found {
		logging.Warn("unknown device — tenant=unassigned", logging.Fields{
			"device_id": deviceID, "topic": topic,
		})
	}

	// Scriere în Influx (cu loguri pe fiecare caz)
	if strings.Contains(topic, "/emeter/0/") {
		valStr := string(payload)
		var value float64
		if _, err := fmt.Sscanf(valStr, "%f", &value); err != nil {
			log.Printf("❌ Eroare conversie la float pentru %s: %v", valStr, err)
			return
		}
		topicParts := strings.Split(topic, "/")
		field := topicParts[len(topicParts)-1]
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "shelly", "type": "power_meter", "tenant_id": tenantTag},
			map[string]interface{}{titleCaser.String(field): value},
			time.Now())
		writePoint(p, writeAPI, topic, payload, logging.Fields{
			"source": "shelly", "field": field, "value": value, "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else if strings.Contains(topic, "/relay/0") {
		valStr := strings.ToLower(string(payload))
		state := 0
		if valStr == "on" {
			state = 1
		}
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "shelly", "type": "relay", "tenant_id": tenantTag},
			map[string]interface{}{"state": state},
			time.Now())
		writePoint(p, writeAPI, topic, payload, logging.Fields{
			"source": "shelly", "type": "relay", "state": state, "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else if strings.HasSuffix(topic, "/STATE") {
		var state StateMessage
		if err := json.Unmarshal(payload, &state); err != nil {
			logging.Drop("parse STATE failed", logging.Fields{"error": err.Error(), "topic": topic, "device_id": deviceID})
			return
		}
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "nousat", "type": "state", "tenant_id": tenantTag},
			map[string]interface{}{"POWER": state.POWER, "RSSI": state.RSSI},
			time.Now())
		writePoint(p, writeAPI, topic, payload, logging.Fields{
			"source": "nousat", "type": "state", "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else if strings.HasSuffix(topic, "/SENSOR") {
		var sensor SensorMessage
		if err := json.Unmarshal(payload, &sensor); err != nil {
			logging.Drop("parse SENSOR failed", logging.Fields{"error": err.Error(), "topic": topic, "device_id": deviceID})
			return
		}
		t, err := time.Parse(time.RFC3339, sensor.Time)
		if err != nil {
			t = time.Now()
		}
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "nousat", "type": "energy", "tenant_id": tenantTag},
			map[string]interface{}{
				"Power":   sensor.ENERGY.Power,
				"Voltage": sensor.ENERGY.Voltage,
				"Current": sensor.ENERGY.Current,
				"Total":   sensor.ENERGY.Total,
			},
			t)
		writePoint(p, writeAPI, topic, payload, logging.Fields{
			"source": "nousat", "type": "energy", "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else if strings.HasPrefix(topic, "zigbee2mqtt/") {
		var data map[string]interface{}
		if err := json.Unmarshal(payload, &data); err != nil {
			logging.Drop("parse zigbee2mqtt failed", logging.Fields{"error": err.Error(), "topic": topic, "device_id": deviceID})
			return
		}
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "zigbee2mqtt", "type": "sensor", "tenant_id": tenantTag},
			data,
			time.Now())
		writePoint(p, writeAPI, topic, payload, logging.Fields{
			"source": "zigbee2mqtt", "type": "sensor", "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else {
		var data map[string]interface{}
		if err := json.Unmarshal(payload, &data); err == nil {
			p := influxdb2.NewPoint("devices",
				map[string]string{"device": deviceID, "source": "generic", "type": "auto_detected", "tenant_id": tenantTag},
				data,
				time.Now())
			writePoint(p, writeAPI, topic, payload, logging.Fields{
				"source": "generic", "type": "auto_detected", "device_id": deviceID, "tenant_id": tenantTag,
			})
		} else {
			valStr := string(payload)
			var val interface{}
			if f, err := strconv.ParseFloat(valStr, 64); err == nil {
				val = f
			} else {
				val = valStr
			}
			p := influxdb2.NewPoint("devices",
				map[string]string{"device": deviceID, "source": "generic", "type": "auto_detected", "tenant_id": tenantTag},
				map[string]interface{}{"value": val},
				time.Now())
			writePoint(p, writeAPI, topic, payload, logging.Fields{
				"source": "generic", "type": "auto_detected_simple", "device_id": deviceID, "tenant_id": tenantTag,
			})
		}
	}
}
