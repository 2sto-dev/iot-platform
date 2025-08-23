package models

type User struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

type AuthResponse struct {
    AccessToken  string `json:"access_token"`
    RefreshToken string `json:"refresh_token"`
}

type RefreshRequest struct {
    RefreshToken string `json:"refresh_token"`
}

type BoilerCurrent struct {
    Device  string  `json:"device"`
    Current float64 `json:"current"`
    Unit    string  `json:"unit"`
}
