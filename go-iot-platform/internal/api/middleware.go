package api

import (
    "context"
    "net/http"
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

var jwtKey = []byte("123456789")

const accessDur = 15 * time.Minute
const refreshDur = 7 * 24 * time.Hour

type contextKey string
const userKey contextKey = "user"

// GenerateTokens rămâne neschimbat (folosit doar pentru login demo)
func GenerateTokens(username string) (string, string, error) {
    ac := jwt.MapClaims{
        "username": username,
        "exp":      time.Now().Add(accessDur).Unix(),
    }
    at := jwt.NewWithClaims(jwt.SigningMethodHS256, ac)
    atStr, err := at.SignedString(jwtKey)
    if err != nil {
        return "", "", err
    }

    rc := jwt.MapClaims{
        "username": username,
        "exp":      time.Now().Add(refreshDur).Unix(),
    }
    rt := jwt.NewWithClaims(jwt.SigningMethodHS256, rc)
    rtStr, err := rt.SignedString(jwtKey)
    if err != nil {
        return "", "", err
    }

    return atStr, rtStr, nil
}

func JWTMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        header := r.Header.Get("Authorization")
        if !strings.HasPrefix(header, "Bearer ") {
            http.Error(w, "Missing token", http.StatusUnauthorized)
            return
        }
        tokenStr := strings.TrimPrefix(header, "Bearer ")
        claims := jwt.MapClaims{}
        token, err := jwt.ParseWithClaims(tokenStr, claims, func(tok *jwt.Token) (interface{}, error) {
            return jwtKey, nil
        })
        if err != nil || !token.Valid {
            http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
            return
        }

        // extragem username din claims
        username, ok := claims["username"].(string)
        if !ok || username == "" {
            http.Error(w, "username missing in token", http.StatusUnauthorized)
            return
        }

        ctx := context.WithValue(r.Context(), userKey, username)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func EnableCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusOK)
            return
        }
        next.ServeHTTP(w, r)
    })
}
