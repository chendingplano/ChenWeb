package Auth

import (
	"encoding/json"
	"log"
	"net/http"
)

// 模拟用户数据库
var users = map[string]string{
	"user@example.com": "123456",
}

type EmailLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type EmailLoginResponse struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func HandleEmailLogin(w http.ResponseWriter, r *http.Request) {
	var req EmailLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("HandleEmailLogin called for (MID_EML_031): %s", req.Email)

	pass, ok := users[req.Email]
	if !ok || pass != req.Password {
		http.Error(w, "invalid email or password (MID_EML_035)", http.StatusUnauthorized)
		return
	}

	resp := EmailLoginResponse{
		Name:  "DeepDoc User",
		Email: req.Email,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
