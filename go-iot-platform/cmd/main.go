package main

import (
    _ "embed"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    mqtt "github.com/eclipse/paho.mqtt.golang"
    influxdb2 "github.com/influxdata/influxdb-client-go/v2"
    influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"

    "go-iot-platform/internal/api"
    "go-iot-platform/internal/influx"
    "go-iot-platform/internal/django"
)

//go:embed go_meeter.log
var initialLog string

func main() {
    fmt.Println("=== LOG ÎNTEGRAT ===")
    fmt.Println(initialLog)

    // Log runtime în consolă + fișier
    if _, err := os.Stat("logs"); os.IsNotExist(err) {
        os.Mkdir("logs", 0755)
    }
    f, _ := os.OpenFile("logs/go_meeter_runtime.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    log.SetOutput(io.MultiWriter(os.Stdout, f))
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

    // 🔑 Login ca superuser (admin)
    if err := django.Login("admin", "egoqwedc/12"); err != nil {
        log.Fatalf("Eroare login Django: %v", err)
    }

    // InfluxDB client
    influxClient := influxdb2.NewClient(influx.URL, influx.Token)
    defer influxClient.Close()
    writeAPI := influxClient.WriteAPIBlocking(influx.Org, influx.Bucket)

    // Pornire MQTT subscriber pentru TOATE device-urile
    go startMQTTSubscriber(writeAPI)

    // Pornire REST API Go
    mux := http.NewServeMux()
    api.RegisterRoutes(mux)
    server := &http.Server{
        Addr:    ":8080",
        Handler: api.EnableCORS(mux),
    }
    log.Println("API server rulând pe http://localhost:8080")
    log.Fatal(server.ListenAndServe())
}

func startMQTTSubscriber(writeAPI influxdb2api.WriteAPIBlocking) {
    var (
        mqttBroker   = "tcp://gate.airweb.ro:1883"
        mqttUsername = "mariea"
        mqttPassword = "mariea"
    )

    // Ia TOATE device-urile din Django
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
                log.Printf("Abonat la topic: %s\n", topic)
            }
        }
    }

    client := mqtt.NewClient(opts)
    if token := client.Connect(); token.Wait() && token.Error() != nil {
        log.Fatalf("Eroare la conectarea MQTT: %v\n", token.Error())
    }
    select {}
}

// Structuri pentru parsare JSON (NousAT)
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

    // --- Shelly EM ---
    if strings.Contains(topic, "/emeter/0/") {
        valStr := string(payload)
        var value float64
        if _, err := fmt.Sscanf(valStr, "%f", &value); err != nil {
            log.Printf("Eroare conversie la float pentru %s: %v", valStr, err)
            return
        }

        parts := strings.Split(topic, "/")
        deviceID := parts[1]
        field := parts[len(parts)-1]

        if field == "power" || field == "voltage" || field == "current" || field == "total" {
            p := influxdb2.NewPoint("devices",
                map[string]string{"device": deviceID, "source": "shelly", "type": "power_meter"},
                map[string]interface{}{strings.Title(field): value},
                time.Now())
            writeAPI.WritePoint(context.Background(), p)
            log.Printf("Scris în InfluxDB (Shelly %s): %.2f", field, value)
        }

    // --- NousAT STATE ---
    } else if strings.HasSuffix(topic, "/STATE") {
        var state StateMessage
        if err := json.Unmarshal(payload, &state); err != nil {
            log.Printf("Eroare parsare STATE: %v", err)
            return
        }
        deviceID := strings.Split(topic, "/")[1]
        p := influxdb2.NewPoint("devices",
            map[string]string{"device": deviceID, "source": "nousat", "type": "state"},
            map[string]interface{}{"POWER": state.POWER, "RSSI": state.RSSI},
            time.Now())
        writeAPI.WritePoint(context.Background(), p)
        log.Printf("Scris în InfluxDB (NousAT STATE): %+v", state)

    // --- NousAT SENSOR ---
    } else if strings.HasSuffix(topic, "/SENSOR") {
        var sensor SensorMessage
        if err := json.Unmarshal(payload, &sensor); err != nil {
            log.Printf("Eroare parsare SENSOR: %v", err)
            return
        }
        t, err := time.Parse(time.RFC3339, sensor.Time)
        if err != nil {
            t = time.Now()
        }
        deviceID := strings.Split(topic, "/")[1]
        p := influxdb2.NewPoint("devices",
            map[string]string{"device": deviceID, "source": "nousat", "type": "energy"},
            map[string]interface{}{
                "Power":   sensor.ENERGY.Power,
                "Voltage": sensor.ENERGY.Voltage,
                "Current": sensor.ENERGY.Current,
                "Total":   sensor.ENERGY.Total,
            },
            t)
        writeAPI.WritePoint(context.Background(), p)
        log.Printf("Scris în InfluxDB (NousAT SENSOR): Power=%.2f W, Voltage=%.1f V, Current=%.2f A, Total=%.3f kWh",
            sensor.ENERGY.Power, sensor.ENERGY.Voltage, sensor.ENERGY.Current, sensor.ENERGY.Total)
    }
}

