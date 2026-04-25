package api

import (
	"log"
	"net/http"
	"strings"

	"go-iot-platform/internal/config"
)

var allowedOrigins = parseAllowedOrigins(config.Get("ALLOWED_ORIGINS"))

func parseAllowedOrigins(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			out[o] = struct{}{}
		}
	}
	if len(out) == 0 {
		log.Println("⚠️ CORS: ALLOWED_ORIGINS gol — toate request-urile cross-origin vor fi respinse")
	} else {
		list := make([]string, 0, len(out))
		for o := range out {
			list = append(list, o)
		}
		log.Printf("CORS allowlist: %s", strings.Join(list, ", "))
	}
	return out
}

// EnableCORS reflect-uiește header-ul Origin doar dacă e în allowlist.
func EnableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowedOrigins[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			}
		}
		if r.Method == http.MethodOptions {
			if _, ok := allowedOrigins[origin]; !ok && origin != "" {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
