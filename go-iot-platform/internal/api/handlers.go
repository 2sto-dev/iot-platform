package api

import (
    "encoding/json"
    "net/http"
    "strings"

    "go-iot-platform/internal/django"
    "go-iot-platform/internal/influx"
)

func RegisterRoutes(mux *http.ServeMux) {
    // JWT login demo – în practică folosești Django
    mux.Handle("/metrics/", JWTMiddleware(http.HandlerFunc(metricsHandler)))
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
    // username din context (extras din JWT)
    username := r.Context().Value(userKey).(string)

    // Ruta e de forma /metrics/{device}/{field}
    path := strings.TrimPrefix(r.URL.Path, "/metrics/")
    segments := strings.Split(path, "/")
    if len(segments) != 2 {
        http.Error(w, "Invalid metric path", http.StatusBadRequest)
        return
    }
    device := segments[0]
    field := segments[1]

    // verificăm device-urile userului din Django
    devices, err := django.GetDevicesForUser(username)
    if err != nil {
        http.Error(w, "Django error: "+err.Error(), http.StatusInternalServerError)
        return
    }
    allowed := false
    for _, d := range devices {
        if d.Serial == device {
            allowed = true
            break
        }
    }
    if !allowed {
        http.Error(w, "Device not allowed for user", http.StatusForbidden)
        return
    }

    // Interogăm Influx
    val, err := influx.GetFieldForDevice(device, field)
    if err != nil {
        http.Error(w, "Influx error: "+err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(map[string]interface{}{
        "device": device,
        "field":  field,
        "value":  val,
    })
}
