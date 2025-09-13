package django

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var (
	baseURL      = getBaseURL()
	accessToken  string
	refreshToken string
)

func getBaseURL() string {
	if v := os.Getenv("DJANGO_BASE_URL"); v != "" {
		return v
	}
	return "http://172.16.0.105:8000/api"
}

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

type RegisterDeviceRequest struct {
	Serial      string `json:"serial_number"`
	Description string `json:"description"`
	DeviceType  string `json:"device_type"`
	ClientID    int    `json:"client"`
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

// 🔄 Refresh token (dacă nu merge → login din nou)
func Refresh() error {
	client := &http.Client{Timeout: 5 * time.Second}
	body, _ := json.Marshal(RefreshRequest{Refresh: refreshToken})
	resp, err := client.Post(baseURL+"/token/refresh/", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		var tok TokenResponse
		if err := json.Unmarshal(data, &tok); err != nil {
			return err
		}
		accessToken = tok.Access
		refreshToken = tok.Refresh
		return nil
	}

	// dacă refresh-ul nu mai e valid → refacem login
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		fmt.Println("⚠️ Refresh invalid, refac login...")
		return Login(os.Getenv("DJANGO_SUPERUSER"), os.Getenv("DJANGO_SUPERPASS"))
	}

	return fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(data))
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

// 📥 Device-urile unui user
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

// 🆕 Înregistrare device automat
func RegisterDevice(dev RegisterDeviceRequest) error {
	client := &http.Client{Timeout: 5 * time.Second}
	body, _ := json.Marshal(dev)
	req, _ := http.NewRequest("POST", baseURL+"/devices/", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		// dacă e Unauthorized → încercăm refresh/login
		if resp.StatusCode == http.StatusUnauthorized {
			if err := Refresh(); err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+accessToken)
			resp, err = client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				data, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("device register failed (%d): %s", resp.StatusCode, string(data))
			}
			return nil
		}
		return fmt.Errorf("device register failed (%d): %s", resp.StatusCode, string(data))
	}
	return nil
}
