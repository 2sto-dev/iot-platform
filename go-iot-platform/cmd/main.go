package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"

	"github.com/joho/godotenv" // pentru încărcarea .env

	"go-iot-platform/internal/api"
	"go-iot-platform/internal/django"
	"go-iot-platform/internal/influx"
)

//go:embed go_meeter.log
var initialLog string

// init rulează înainte de main()
func init() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Nu s-a putut încărca .env, folosesc doar variabilele din sistem")
	}
}

func main() {
	fmt.Println("=== LOG INTEGRAT ===")
	fmt.Println(initialLog)

	// 📂 log runtime în consolă + fișier
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		_ = os.Mkdir("logs", 0755)
	}
	f, _ := os.OpenFile("logs/go_meeter_runtime.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 🔑 Login ca superuser (admin Django)
	if err := django.Login(os.Getenv("DJANGO_SUPERUSER"), os.Getenv("DJANGO_SUPERPASS")); err != nil {
		log.Fatalf("Eroare login Django: %v", err)
	}

	// InfluxDB client
	influxClient := influxdb2.NewClient(influx.URL, influx.Token)
	defer influxClient.Close()
	writeAPI := influxClient.WriteAPIBlocking(influx.Org, influx.Bucket)

	// Pornire MQTT subscriber
	go startMQTTSubscriber(writeAPI)

	// Pornire REST API Go
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// fallback pentru orice request → log + 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("⚠️ Request necunoscut: %s %s", r.Method, r.URL.Path)
		log.Printf("⚠️ Headers: %+v", r.Header)
		http.NotFound(w, r)
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: api.EnableCORS(http.StripPrefix("/go", mux)),
	}

	log.Println("✅ API Go disponibil pe http://localhost:8080/go/* prin Kong")
	log.Fatal(server.ListenAndServe())
}

func startMQTTSubscriber(writeAPI influxdb2api.WriteAPIBlocking) {
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
	select {}
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

	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return
	}
	deviceID := parts[1]

	devices, _ := django.GetAllDevices()
	found := false
	for _, d := range devices {
		if d.Serial == deviceID {
			found = true
			break
		}
	}

	if !found {
		devType := "auto_detected"
		if strings.HasPrefix(topic, "zigbee2mqtt/") {
			devType = "zigbee_sensor"
		} else if strings.Contains(topic, "emeter") {
			devType = "shelly_em"
		} else if strings.Contains(topic, "STATE") || strings.Contains(topic, "SENSOR") {
			devType = "nous_at"
		}

		devReq := django.RegisterDeviceRequest{
			Serial:      deviceID,
			Description: fmt.Sprintf("Auto-registered from topic %s", topic),
			DeviceType:  devType,
			ClientID:    1,
		}
		if err := django.RegisterDevice(devReq); err != nil {
			log.Printf("❌ Eroare la înregistrarea device-ului %s: %v", deviceID, err)
		} else {
			log.Printf("✅ Device %s înregistrat automat în Django (%s)", deviceID, devType)
		}
	}

	// Scriere în Influx (cu loguri pe fiecare caz)
	if strings.Contains(topic, "/emeter/0/") {
		valStr := string(payload)
		var value float64
		if _, err := fmt.Sscanf(valStr, "%f", &value); err != nil {
			log.Printf("❌ Eroare conversie la float pentru %s: %v", valStr, err)
			return
		}
		field := parts[len(parts)-1]
		p := influxdb2.NewPoint("devices",
			map[string]string{"device": deviceID, "source": "shelly", "type": "power_meter"},
			map[string]interface{}{strings.Title(field): value},
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
			map[string]string{"device": deviceID, "source": "shelly", "type": "relay"},
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
			map[string]string{"device": deviceID, "source": "nousat", "type": "state"},
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
			map[string]string{"device": deviceID, "source": "nousat", "type": "energy"},
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
			map[string]string{"device": deviceID, "source": "zigbee2mqtt", "type": "sensor"},
			data,
			time.Now())
		_ = writeAPI.WritePoint(context.Background(), p)
		log.Printf("📊 Scris în InfluxDB (Zigbee2MQTT %s): %+v", deviceID, data)

	} else {
		var data map[string]interface{}
		if err := json.Unmarshal(payload, &data); err == nil {
			p := influxdb2.NewPoint("devices",
				map[string]string{"device": deviceID, "source": "generic", "type": "auto_detected"},
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
				map[string]string{"device": deviceID, "source": "generic", "type": "auto_detected"},
				map[string]interface{}{"value": val},
				time.Now())
			_ = writeAPI.WritePoint(context.Background(), p)
			log.Printf("📊 Scris în InfluxDB (Generic simplu %s): %v", deviceID, val)
		}
	}
}
