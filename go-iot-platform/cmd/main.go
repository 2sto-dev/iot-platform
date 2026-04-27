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
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"go-iot-platform/internal/api"
	"go-iot-platform/internal/django"
	"go-iot-platform/internal/influx"
	"go-iot-platform/internal/topics"
)

var titleCaser = cases.Title(language.Und)

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

func handleMessage(msg mqtt.Message, writeAPI influxdb2api.WriteAPIBlocking) {
	topic := msg.Topic()
	payload := msg.Payload()

	log.Printf("📥 MQTT Message: topic=%s payload=%s", topic, string(payload))

	// Parse topic: dacă începe cu "tenants/", schema nouă cu validare strictă;
	// altfel legacy → continuăm flow-ul existent (lookup device→tenant via Django).
	parsed, err := topics.Parse(topic)
	if err != nil {
		log.Printf("⛔ TOPIC INVALID — DROP: %v", err)
		return
	}

	var deviceID string
	if parsed.IsLegacy {
		deviceID = topics.LegacyDeviceID(topic)
	} else {
		deviceID = parsed.DeviceID
	}
	// #7 enforcement: device_id e obligatoriu — fără el nu putem tag-a punctul
	if deviceID == "" {
		log.Printf("⛔ EMPTY device_id (topic=%s) — DROP", topic)
		return
	}

	devices, _ := django.GetAllDevices()
	tenantTag := "unassigned"
	var deviceTenantID int64
	found := false
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

	// #4 Validare device ↔ tenant: dacă topic-ul declară un tenant explicit (schema nouă),
	// trebuie să corespundă cu tenantul real al device-ului din Django. Mismatch → DROP.
	if !parsed.IsLegacy {
		if !found {
			log.Printf("⛔ Device %s nu există în Django (topic %s) — DROP (schema nouă cere device înregistrat)", deviceID, topic)
			return
		}
		if deviceTenantID != parsed.TenantID {
			log.Printf("⛔ DEVICE-TENANT MISMATCH device=%s topic_tenant=%d device_tenant=%d — DROP",
				deviceID, parsed.TenantID, deviceTenantID)
			return
		}
	}

	if !found {
		log.Printf("⚠️ Device necunoscut %s (topic %s) — telemetrie marcată tenant_id=unassigned, NU se mai auto-înregistrează", deviceID, topic)
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
		_ = writeAPI.WritePoint(context.Background(), p)
		log.Printf("📊 Scris în InfluxDB (Shelly %s): %.2f", field, value)

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
		_ = writeAPI.WritePoint(context.Background(), p)
		log.Printf("📊 Scris în InfluxDB (Shelly relay state): %d", state)

	} else if strings.HasSuffix(topic, "/STATE") {
		var state StateMessage
		if err := json.Unmarshal(payload, &state); err != nil {
			log.Printf("❌ Eroare parsare STATE: %v", err)
			return
		}
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "nousat", "type": "state", "tenant_id": tenantTag},
			map[string]interface{}{"POWER": state.POWER, "RSSI": state.RSSI},
			time.Now())
		_ = writeAPI.WritePoint(context.Background(), p)
		log.Printf("📊 Scris în InfluxDB (NousAT STATE): %+v", state)

	} else if strings.HasSuffix(topic, "/SENSOR") {
		var sensor SensorMessage
		if err := json.Unmarshal(payload, &sensor); err != nil {
			log.Printf("❌ Eroare parsare SENSOR: %v", err)
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
		_ = writeAPI.WritePoint(context.Background(), p)
		log.Printf("📊 Scris în InfluxDB (NousAT SENSOR): %+v", sensor.ENERGY)

	} else if strings.HasPrefix(topic, "zigbee2mqtt/") {
		var data map[string]interface{}
		if err := json.Unmarshal(payload, &data); err != nil {
			log.Printf("❌ Eroare parsare Zigbee payload: %v", err)
			return
		}
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "zigbee2mqtt", "type": "sensor", "tenant_id": tenantTag},
			data,
			time.Now())
		_ = writeAPI.WritePoint(context.Background(), p)
		log.Printf("📊 Scris în InfluxDB (Zigbee2MQTT %s): %+v", deviceID, data)

	} else {
		var data map[string]interface{}
		if err := json.Unmarshal(payload, &data); err == nil {
			p := influxdb2.NewPoint("devices",
				map[string]string{"device": deviceID, "source": "generic", "type": "auto_detected", "tenant_id": tenantTag},
				data,
				time.Now())
			_ = writeAPI.WritePoint(context.Background(), p)
			log.Printf("📊 Scris în InfluxDB (Generic JSON %s): %+v", deviceID, data)
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
			_ = writeAPI.WritePoint(context.Background(), p)
			log.Printf("📊 Scris în InfluxDB (Generic simplu %s): %v", deviceID, val)
		}
	}
}
