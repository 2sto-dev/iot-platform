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

	// Faza 2.5: WriteAPI async cu batching; Faza 2.7: pool de WriteAPI per plan de tenant.
	opts := influxdb2.DefaultOptions().
		SetBatchSize(5000).
		SetFlushInterval(1000) // ms
	influxClient := influxdb2.NewClientWithOptions(influx.URL, influx.Token, opts)
	defer influxClient.Close()

	poolErrCh := make(chan error, 32)
	writePool := influx.NewWritePool(influxClient, influx.Org, influx.BucketConfig{
		Free:       os.Getenv("INFLUX_BUCKET_FREE"),
		Pro:        os.Getenv("INFLUX_BUCKET_PRO"),
		Enterprise: os.Getenv("INFLUX_BUCKET_ENTERPRISE"),
	}, poolErrCh)
	go func() {
		for err := range poolErrCh {
			logging.Error("influx async write error", logging.Fields{"error": err.Error()})
			if influxBuffer != nil {
				_ = influxBuffer.Append("(async batch)", []byte("(batch error)"), err)
			}
		}
	}()

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

	go startMQTTSubscriber(ctx, writePool)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("⚠️ Request necunoscut: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "8090"
	}
	server := &http.Server{
		Addr:    "0.0.0.0:" + apiPort,
		Handler: api.EnableCORS(http.StripPrefix("/go", mux)),
	}

	go func() {
		log.Printf("✅ API Go disponibil pe http://localhost:%s/go/*", apiPort)
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

func startMQTTSubscriber(ctx context.Context, pool *influx.WritePool) {
	mqttBroker := os.Getenv("MQTT_BROKER")
	mqttUsername := os.Getenv("MQTT_USER")
	mqttPassword := os.Getenv("MQTT_PASS")

	if mqttBroker == "" {
		log.Fatal("⚠️ MQTT_BROKER nu este setat în .env")
	}

	// Faza 2.3: shared subscription pe schema nouă tenant-aware. EMQX distribuie mesajele
	// load-balanced între instanțele care se abonează cu același share name "ingest" → poți
	// rula N instanțe Go fără duplicare.
	//
	// Topicuri legacy (vendor-shaped) sunt covered separat de bridge (Faza 2.2) sau, până
	// atunci, de un fallback pe pattern-urile cunoscute. Wildcard "#" eliminat — era
	// risc de a primi tot ce trece prin broker, inclusiv noise/control plane MQTT.
	clientID := os.Getenv("MQTT_CLIENT_ID")
	if clientID == "" {
		clientID = fmt.Sprintf("go-ingest-%d", time.Now().UnixNano())
	}

	subscriptions := []string{
		"$share/ingest/tenants/+/devices/+/up/#", // schema nouă (Faza 2.1)
		"$share/ingest/tenants/+/devices/+/up/cmd_ack", // Faza 3.3 ACK downlink
		// Legacy fallback patterns — eliminate când Faza 2.2 (bridge) e activ în prod.
		"$share/ingest-legacy/shellies/+/#",
		"$share/ingest-legacy/tele/+/#",
		"$share/ingest-legacy/zigbee2mqtt/+",
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttBroker)
	opts.SetUsername(mqttUsername)
	opts.SetPassword(mqttPassword)
	opts.SetClientID(clientID)
	opts.SetCleanSession(true)

	opts.OnConnect = func(c mqtt.Client) {
		for _, topic := range subscriptions {
			if token := c.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
				go handleMessage(msg, pool)
			}); token.Wait() && token.Error() != nil {
				log.Printf("Eroare la abonare topic %s: %v\n", topic, token.Error())
			} else {
				log.Printf("✅ Abonat la (shared): %s", topic)
			}
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
	Total          float64 `json:"Total"`
	Today          float64 `json:"Today"`
	Yesterday      float64 `json:"Yesterday"`
	Power          float64 `json:"Power"`
	ApparentPower  float64 `json:"ApparentPower"`
	ReactivePower  float64 `json:"ReactivePower"`
	Factor         float64 `json:"Factor"`
	Voltage        float64 `json:"Voltage"`
	Current        float64 `json:"Current"`
}

type SensorMessage struct {
	Time   string     `json:"Time"`
	ENERGY EnergyData `json:"ENERGY"`
}

// writePoint scrie un punct în Influx pe bucket-ul planului dat. Loghează enqueue-ul structurat.
func writePoint(p *write.Point, pool *influx.WritePool, plan string, fields logging.Fields) {
	pool.WritePoint(plan, p)
	logging.Info("influx write enqueued", fields)
}

func handleMessage(msg mqtt.Message, pool *influx.WritePool) {
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

	// Faza 2.4 + 2.7: lookup device→tenant+plan via Redis cache sau fallback Django.
	tenantTag := "unassigned"
	tenantPlan := "free"
	var deviceTenantID int64
	found := false
	if deviceCache != nil {
		if entry, ok := deviceCache.GetDeviceInfo(context.Background(), deviceID); ok {
			found = true
			deviceTenantID = entry.TenantID
			tenantTag = cache.ParseTenantTag(entry.TenantID)
			if entry.TenantPlan != "" {
				tenantPlan = entry.TenantPlan
			}
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
				if d.TenantPlan != "" {
					tenantPlan = d.TenantPlan
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

	// SUN2000 (și alte device-uri cu stream "telemetry"): payload cu array measurements
	if parsed.Stream == "telemetry" {
		var sun struct {
			Ts           string                   `json:"ts"`
			Measurements []map[string]interface{} `json:"measurements"`
			HouseLoad    float64                  `json:"house_load_kw_est"`
		}
		if err := json.Unmarshal(payload, &sun); err == nil && len(sun.Measurements) > 0 {
			fields := make(map[string]interface{}, len(sun.Measurements)+1)
			for _, m := range sun.Measurements {
				if key, ok := m["key"].(string); ok {
					if val, ok := m["value"]; ok {
						fields[key] = val
					}
				}
			}
			if sun.HouseLoad != 0 {
				fields["house_load_kw_est"] = sun.HouseLoad
			}
			t := time.Now()
			if sun.Ts != "" {
				if pt, err := time.Parse(time.RFC3339, sun.Ts); err == nil {
					t = pt
				}
			}
			p := influxdb2.NewPoint("devices",
				map[string]string{"device": deviceID, "source": "sun2000", "type": "solar_inverter", "tenant_id": tenantTag},
				fields, t)
			writePoint(p, pool, tenantPlan, logging.Fields{
				"source": "sun2000", "type": "solar_inverter", "device_id": deviceID, "tenant_id": tenantTag,
			})
		}
		return
	}

	// Faza 3.3: ACK pentru comenzi downlink
	if strings.HasSuffix(topic, "/up/cmd_ack") || parsed.Stream == "cmd_ack" {
		var ack struct {
			CommandID int64          `json:"command_id"`
			Success   bool           `json:"success"`
			Result    map[string]any `json:"result"`
		}
		if err := json.Unmarshal(payload, &ack); err != nil {
			logging.Drop("cmd_ack parse failed", logging.Fields{"error": err.Error(), "device_id": deviceID})
			return
		}
		cmdStatus := "executed"
		if !ack.Success {
			cmdStatus = "failed"
		}
		if err := django.AckCommand(ack.CommandID, cmdStatus, ack.Result); err != nil {
			logging.Warn("AckCommand failed", logging.Fields{"cmd_id": ack.CommandID, "error": err.Error()})
		}
		return
	}

	// Faza 3.5: OTA status raportat de device
	if strings.HasSuffix(topic, "/up/ota") || parsed.Stream == "ota" {
		var ota struct {
			FirmwareID int64  `json:"firmware_id"`
			Status     string `json:"status"`
			Error      string `json:"error"`
		}
		if err := json.Unmarshal(payload, &ota); err != nil {
			logging.Drop("ota parse failed", logging.Fields{"error": err.Error(), "device_id": deviceID})
			return
		}
		if err := django.UpdateOTAStatus(deviceID, ota.FirmwareID, ota.Status, ota.Error); err != nil {
			logging.Warn("UpdateOTAStatus failed", logging.Fields{"device_id": deviceID, "error": err.Error()})
		}
		return
	}

	// Faza 3.4: shadow reported de la device
	if strings.HasSuffix(topic, "/up/shadow") || parsed.Stream == "shadow" {
		var reported map[string]interface{}
		if err := json.Unmarshal(payload, &reported); err != nil {
			logging.Drop("shadow parse failed", logging.Fields{"error": err.Error(), "device_id": deviceID})
			return
		}
		if err := django.UpdateShadowReported(deviceID, reported); err != nil {
			logging.Warn("UpdateShadowReported failed", logging.Fields{"device_id": deviceID, "error": err.Error()})
		}
		return
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
		writePoint(p, pool, tenantPlan, logging.Fields{
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
		writePoint(p, pool, tenantPlan, logging.Fields{
			"source": "shelly", "type": "relay", "state": state, "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else if strings.HasSuffix(strings.ToLower(topic), "/state") {
		// Tasmota STATE — accepta atat /STATE original cat si /state lowercase (bridge).
		// IMPORTANT: NU scriem `power` aici (conflict de tip cu SENSOR power=float).
		// Folosim `relay_state` (string ON/OFF) pentru releu si pastram `rssi` numeric.
		var state StateMessage
		if err := json.Unmarshal(payload, &state); err != nil {
			logging.Drop("parse STATE failed", logging.Fields{"error": err.Error(), "topic": topic, "device_id": deviceID})
			return
		}
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "nousat", "type": "state", "tenant_id": tenantTag},
			map[string]interface{}{"relay_state": state.POWER, "rssi": state.RSSI},
			time.Now())
		writePoint(p, pool, tenantPlan, logging.Fields{
			"source": "nousat", "type": "state", "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else if strings.HasSuffix(strings.ToLower(topic), "/sensor") {
		// Tasmota SENSOR — atât topic original /SENSOR cât și schema noua /sensor (translate de bridge).
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
				"power":          sensor.ENERGY.Power,
				"apparent_power": sensor.ENERGY.ApparentPower,
				"reactive_power": sensor.ENERGY.ReactivePower,
				"power_factor":   sensor.ENERGY.Factor,
				"voltage":        sensor.ENERGY.Voltage,
				"current":        sensor.ENERGY.Current,
				"total":          sensor.ENERGY.Total,
				"today":          sensor.ENERGY.Today,
				"yesterday":      sensor.ENERGY.Yesterday,
			},
			t)
		writePoint(p, pool, tenantPlan, logging.Fields{
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
		writePoint(p, pool, tenantPlan, logging.Fields{
			"source": "zigbee2mqtt", "type": "sensor", "device_id": deviceID, "tenant_id": tenantTag,
		})

	} else {
		var data map[string]interface{}
		if err := json.Unmarshal(payload, &data); err == nil {
			p := influxdb2.NewPoint("devices",
				map[string]string{"device": deviceID, "source": "generic", "type": "auto_detected", "tenant_id": tenantTag},
				data,
				time.Now())
			writePoint(p, pool, tenantPlan, logging.Fields{
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
			writePoint(p, pool, tenantPlan, logging.Fields{
				"source": "generic", "type": "auto_detected_simple", "device_id": deviceID, "tenant_id": tenantTag,
			})
		}
	}
}
