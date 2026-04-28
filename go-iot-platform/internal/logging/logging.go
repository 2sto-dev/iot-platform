// Package logging emite log-uri JSON line-per-line cu câmpuri structurate.
//
// Câmpuri obligatorii la fiecare event de message-handling: tenant_id, device_id, topic.
// User_id apare în log-urile de API (extras din JWT). Util pentru Loki / ELK.
package logging

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

type Fields map[string]interface{}

var (
	mu     sync.Mutex
	logger = log.New(os.Stdout, "", 0)
)

// SetOutput permite suprascrierea destinației (util în teste sau combinat cu io.MultiWriter).
func SetOutput(w *log.Logger) {
	mu.Lock()
	defer mu.Unlock()
	logger = w
}

func emit(level, msg string, f Fields) {
	if f == nil {
		f = Fields{}
	}
	f["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	f["level"] = level
	f["msg"] = msg
	b, err := json.Marshal(f)
	if err != nil {
		// ultimă linie de apărare: log unstructured
		log.Printf("logging.emit marshal error: %v (msg=%q)", err, msg)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	logger.Println(string(b))
}

func Info(msg string, f Fields)  { emit("info", msg, f) }
func Warn(msg string, f Fields)  { emit("warn", msg, f) }
func Error(msg string, f Fields) { emit("error", msg, f) }
func Drop(msg string, f Fields)  { emit("drop", msg, f) }
