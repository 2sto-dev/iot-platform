package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

func init() {
	loadEnv()
}

func loadEnv() {
	candidates := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, ".env"),
			filepath.Join(dir, "..", ".env"),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			if err := godotenv.Load(p); err == nil {
				log.Printf("config: loaded env from %s", p)
				return
			}
		}
	}
	log.Println("config: no .env file found, using process environment")
}

type Config struct {
	MQTTBroker  string
	MQTTPort    string
	InfluxURL   string
	InfluxToken string
}

func LoadConfig() *Config {
	return &Config{
		MQTTBroker:  getEnv("MQTT_BROKER", "localhost"),
		MQTTPort:    getEnv("MQTT_PORT", "1883"),
		InfluxURL:   getEnv("INFLUX_URL", "http://localhost:8086"),
		InfluxToken: getEnv("INFLUX_TOKEN", ""),
	}
}

func Get(key string) string {
	return os.Getenv(key)
}

func getEnv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}
