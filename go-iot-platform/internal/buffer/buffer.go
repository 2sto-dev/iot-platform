// Package buffer scrie append-only într-un fișier când Influx pică.
//
// Limite cunoscute (intenționate):
// - fără rotație: caller-ul trebuie să monitorizeze size-ul și să arhiveze periodic
// - fără replay automat: re-ingestia se face cu un script offline (TBD în Faza 2.5)
// - sync per linie nu se face pentru performanță; pierdere posibilă pe crash între
//   write și fsync. Acceptat — fallback-ul protejează numai de eșecuri ale Influx, nu
//   de eșecuri ale procesului.
package buffer

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type Entry struct {
	Topic     string    `json:"topic"`
	Payload   string    `json:"payload"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"ts"`
}

type FileBuffer struct {
	mu sync.Mutex
	f  *os.File
}

func New(path string) (*FileBuffer, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open buffer file: %w", err)
	}
	return &FileBuffer{f: f}, nil
}

func (b *FileBuffer) Append(topic string, payload []byte, cause error) error {
	if b == nil {
		return nil
	}
	e := Entry{
		Topic:     topic,
		Payload:   string(payload),
		Error:     cause.Error(),
		Timestamp: time.Now().UTC(),
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("buffer marshal: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := b.f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("buffer write: %w", err)
	}
	return nil
}

func (b *FileBuffer) Close() error {
	if b == nil || b.f == nil {
		return nil
	}
	return b.f.Close()
}
