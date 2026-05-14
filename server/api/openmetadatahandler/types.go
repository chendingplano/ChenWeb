package openmetadatahandler

type config struct {
	UpstreamURL    string
	PublicBasePath string
	DisplayName    string
}

type CurrentUser struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type SessionResponse struct {
	Status        bool     `json:"status"`
	LaunchURL     string   `json:"launch_url"`
	ProxyBasePath string   `json:"proxy_base_path"`
	DisplayName   string   `json:"display_name"`
	UserID        string   `json:"user_id"`
	Capabilities  []string `json:"capabilities"`
	Message       string   `json:"message,omitempty"`
}

type ErrorResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}
