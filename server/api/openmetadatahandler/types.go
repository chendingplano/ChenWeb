package openmetadatahandler

type config struct {
	UpstreamURL    string
	PublicBasePath string
	DisplayName    string
	SSOMode        string
	BearerToken    string
}

type CurrentUser struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type SessionResponse struct {
	Status           bool     `json:"status"`
	LaunchURL        string   `json:"launch_url"`
	ProxyBasePath    string   `json:"proxy_base_path"`
	CallbackURL      string   `json:"callback_url,omitempty"`
	DisplayName      string   `json:"display_name"`
	UserID           string   `json:"user_id"`
	SSOMode          string   `json:"sso_mode"`
	Capabilities     []string `json:"capabilities"`
	AuthBoundaryNote string   `json:"auth_boundary_note,omitempty"`
	Message          string   `json:"message,omitempty"`
}

type ErrorResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}
