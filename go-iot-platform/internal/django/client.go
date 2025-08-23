package django

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

var (
    baseURL      = "http://localhost:8000/api"
    accessToken  string
    refreshToken string
)

type LoginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

type TokenResponse struct {
    Access  string `json:"access"`
    Refresh string `json:"refresh"`
}

type RefreshRequest struct {
    Refresh string `json:"refresh"`
}

type Device struct {
    Serial string   `json:"serial_number"`
    Topics []string `json:"topics"`
}

// 🔑 Login la Django
func Login(username, password string) error {
    client := &http.Client{Timeout: 5 * time.Second}
    body, _ := json.Marshal(LoginRequest{Username: username, Password: password})
    resp, err := client.Post(baseURL+"/token/", "application/json", bytes.NewBuffer(body))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    data, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("login failed (%d): %s", resp.StatusCode, string(data))
    }

    var tok TokenResponse
    if err := json.Unmarshal(data, &tok); err != nil {
        return err
    }

    accessToken = tok.Access
    refreshToken = tok.Refresh
    return nil
}

// 🔄 Refresh token
func Refresh() error {
    client := &http.Client{Timeout: 5 * time.Second}
    body, _ := json.Marshal(RefreshRequest{Refresh: refreshToken})
    resp, err := client.Post(baseURL+"/token/refresh/", "application/json", bytes.NewBuffer(body))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    data, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(data))
    }

    var tok TokenResponse
    if err := json.Unmarshal(data, &tok); err != nil {
        return err
    }

    accessToken = tok.Access
    refreshToken = tok.Refresh
    return nil
}

// 📥 Toate device-urile (superuser)
func GetAllDevices() ([]Device, error) {
    client := &http.Client{Timeout: 5 * time.Second}
    req, _ := http.NewRequest("GET", baseURL+"/devices/", nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)

    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusUnauthorized {
        if err := Refresh(); err != nil {
            return nil, err
        }
        req.Header.Set("Authorization", "Bearer "+accessToken)
        resp, err = client.Do(req)
        if err != nil {
            return nil, err
        }
        defer resp.Body.Close()
    }

    data, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("error fetching all devices (%d): %s", resp.StatusCode, string(data))
    }

    var devs []Device
    if err := json.Unmarshal(data, &devs); err != nil {
        return nil, err
    }
    return devs, nil
}

// 📥 Device-urile unui singur user (pentru API metrics)
func GetDevicesForUser(username string) ([]Device, error) {
    client := &http.Client{Timeout: 5 * time.Second}
    req, _ := http.NewRequest("GET", fmt.Sprintf("%s/devices/%s/", baseURL, username), nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)

    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusUnauthorized {
        if err := Refresh(); err != nil {
            return nil, err
        }
        req.Header.Set("Authorization", "Bearer "+accessToken)
        resp, err = client.Do(req)
        if err != nil {
            return nil, err
        }
        defer resp.Body.Close()
    }

    data, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("error fetching devices for user %s (%d): %s", username, resp.StatusCode, string(data))
    }

    var devs []Device
    if err := json.Unmarshal(data, &devs); err != nil {
        return nil, err
    }
    return devs, nil
}
