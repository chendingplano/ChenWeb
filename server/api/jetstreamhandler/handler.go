package jetstreamhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"
)

var allowedEndpoints = map[string]bool{
	"jsz":   true,
	"varz":  true,
	"connz": true,
}

func monitorBaseURL() string {
	explicit := strings.TrimSpace(os.Getenv("NATS_MONITOR_URL"))
	if explicit != "" {
		return strings.TrimSuffix(explicit, "/")
	}

	host := strings.TrimSpace(os.Getenv("NATS_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("NATS_HTTP_PORT"))
	if port == "" {
		port = "8222"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func natsURL() string {
	explicit := strings.TrimSpace(os.Getenv("NATS_URL"))
	if explicit != "" {
		return explicit
	}

	host := strings.TrimSpace(os.Getenv("NATS_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("NATS_PORT"))
	if port == "" {
		port = "4222"
	}
	return fmt.Sprintf("nats://%s:%s", host, port)
}

func natsConnectOptions() ([]nats.Option, string) {
	opts := []nats.Option{
		nats.Timeout(6 * time.Second),
	}

	token := strings.TrimSpace(os.Getenv("NATS_TOKEN"))
	user := strings.TrimSpace(os.Getenv("NATS_USER"))
	pass := strings.TrimSpace(os.Getenv("NATS_PASS"))
	if pass == "" {
		pass = strings.TrimSpace(os.Getenv("NATS_PASSWORD"))
	}
	credsFile := strings.TrimSpace(os.Getenv("NATS_CREDS_FILE"))
	if credsFile == "" {
		credsFile = strings.TrimSpace(os.Getenv("NATS_CREDS"))
	}

	if credsFile != "" {
		opts = append(opts, nats.UserCredentials(credsFile))
		return opts, "creds"
	}
	if token != "" {
		opts = append(opts, nats.Token(token))
		return opts, "token"
	}
	if user != "" || pass != "" {
		opts = append(opts, nats.UserInfo(user, pass))
		return opts, "userpass"
	}
	return opts, "none"
}

// GetMonitor handles GET /api/v1/jetstream/monitor?endpoint=jsz|varz|connz
func GetMonitor(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_JSH_100")
	defer rc.Close()
	logger := rc.GetLogger()

	endpoint := strings.ToLower(strings.TrimSpace(c.QueryParam("endpoint")))
	if endpoint == "" {
		endpoint = "jsz"
	}
	if !allowedEndpoints[endpoint] {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": fmt.Sprintf("unsupported endpoint '%s'; use jsz|varz|connz", endpoint),
		})
	}

	target := monitorBaseURL() + "/" + endpoint
	client := &http.Client{Timeout: 8 * time.Second}

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		logger.Error("failed to build monitor request", "target", target, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to build monitor request",
		})
	}
	req.Header.Set("Accept", "application/json,text/plain;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to reach monitor endpoint", "target", target, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]any{
			"ok":      false,
			"message": "failed to reach JetStream monitor endpoint",
			"target":  target,
			"error":   err.Error(),
		})
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		logger.Error("failed reading monitor response", "target", target, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]any{
			"ok":      false,
			"message": "failed to read JetStream monitor response",
			"target":  target,
		})
	}

	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		// Keep plaintext payload if response is not JSON.
		data = string(raw)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.JSON(http.StatusBadGateway, map[string]any{
			"ok":       false,
			"message":  fmt.Sprintf("JetStream monitor returned %d", resp.StatusCode),
			"target":   target,
			"endpoint": endpoint,
			"data":     data,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"target":   target,
		"endpoint": endpoint,
		"data":     data,
	})
}

type PublishEventRequest struct {
	Subject string `json:"subject"`
	Payload string `json:"payload"`
}

func validateSubjectPayload(subject string, payload string) error {
	requiredFields := map[string][]string{
		"kb.pdf.parsed":          {"record_id", "type", "status"},
		"kb.pdf.staged":          {"record_id", "type", "status"},
		"kb.line-file-generated": {"record_id", "type", "status"},
	}
	fields, ok := requiredFields[subject]
	if !ok {
		return nil
	}

	body := strings.TrimSpace(payload)
	if body == "" {
		return fmt.Errorf("payload is required for subject %s", subject)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return fmt.Errorf("payload must be valid JSON object for subject %s", subject)
	}

	for _, key := range fields {
		value, ok := decoded[key]
		if !ok || value == nil {
			return fmt.Errorf("payload field '%s' is required for subject %s", key, subject)
		}
		switch key {
		case "type", "status":
			v, ok := value.(string)
			if !ok || strings.TrimSpace(v) == "" {
				return fmt.Errorf("payload field '%s' must be a non-empty string for subject %s", key, subject)
			}
		}
	}
	if force, ok := decoded["force"]; ok && force != nil {
		if _, ok := force.(bool); !ok {
			return fmt.Errorf("payload field 'force' must be boolean for subject %s", subject)
		}
	}

	return nil
}

type StoredSubject struct {
	ID            int64     `json:"id"`
	Subject       string    `json:"subject"`
	Description   *string   `json:"description,omitempty"`
	PayloadFormat *string   `json:"payload_format,omitempty"`
	IsActive      bool      `json:"is_active"`
	CreatedBy     *string   `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateStoredSubjectRequest struct {
	Subject       string `json:"subject"`
	Description   string `json:"description"`
	PayloadFormat string `json:"payload_format"`
}

type UpdateStoredSubjectRequest struct {
	Subject       string `json:"subject"`
	Description   string `json:"description"`
	PayloadFormat string `json:"payload_format"`
	IsActive      bool   `json:"is_active"`
}

func getSharedDBHandle() *sql.DB {
	if ApiTypes.SharedDBHandle != nil {
		return ApiTypes.SharedDBHandle
	}
	if ApiTypes.SharedMigrationDBHandle != nil {
		return ApiTypes.SharedMigrationDBHandle
	}
	return nil
}

// GetSubjects handles GET /api/v1/jetstream/subjects
func GetSubjects(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_JSH_130")
	defer rc.Close()
	logger := rc.GetLogger()

	db := getSharedDBHandle()
	if db == nil {
		logger.Error("shared db handle is nil")
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "database connection is not initialized",
		})
	}

	rows, err := db.Query(`
SELECT subject
FROM shared.nats_subjects
WHERE is_active = true
ORDER BY subject ASC
`)
	if err != nil {
		logger.Error("failed to list active nats subjects", "error", err)
		if strings.Contains(err.Error(), "42P01") || strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"ok":      false,
				"message": "table shared.nats_subjects is missing; run shared migrations and restart server",
				"error":   err.Error(),
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to list nats subjects",
		})
	}
	defer rows.Close()

	subjects := make([]string, 0, 16)
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			logger.Error("failed to scan nats subject", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"ok":      false,
				"message": "failed to parse nats subjects",
			})
		}
		subject = strings.TrimSpace(subject)
		if subject == "" {
			continue
		}
		subjects = append(subjects, subject)
	}

	if err := rows.Err(); err != nil {
		logger.Error("failed iterating nats subjects", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to list nats subjects",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"subjects": subjects,
	})
}

// ListStoredSubjects handles GET /api/v1/jetstream/nats-subjects
func ListStoredSubjects(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_JSH_160")
	defer rc.Close()
	logger := rc.GetLogger()

	db := getSharedDBHandle()
	if db == nil {
		logger.Error("shared db handle is nil")
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "database connection is not initialized",
		})
	}

	rows, err := db.Query(`
SELECT id, subject, description, payload_format, is_active, created_by, created_at, updated_at
FROM shared.nats_subjects
ORDER BY subject ASC
`)
	if err != nil {
		logger.Error("failed to list nats subjects", "error", err)
		if strings.Contains(err.Error(), "42P01") || strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"ok":      false,
				"message": "table shared.nats_subjects is missing; run shared migrations and restart server",
				"error":   err.Error(),
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to list nats subjects",
		})
	}
	defer rows.Close()

	out := make([]StoredSubject, 0)
	for rows.Next() {
		var s StoredSubject
		if err := rows.Scan(
			&s.ID,
			&s.Subject,
			&s.Description,
			&s.PayloadFormat,
			&s.IsActive,
			&s.CreatedBy,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			logger.Error("failed to scan nats subject", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"ok":      false,
				"message": "failed to parse nats subjects",
			})
		}
		out = append(out, s)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":       true,
		"subjects": out,
	})
}

// CreateStoredSubject handles POST /api/v1/jetstream/nats-subjects
func CreateStoredSubject(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_JSH_170")
	defer rc.Close()
	logger := rc.GetLogger()

	db := getSharedDBHandle()
	if db == nil {
		logger.Error("shared db handle is nil")
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "database connection is not initialized",
		})
	}

	req := CreateStoredSubjectRequest{}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "invalid request payload",
		})
	}

	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "subject is required",
		})
	}

	var createdBy *string
	if userInfo := rc.IsAuthenticated(); userInfo != nil && strings.TrimSpace(userInfo.Email) != "" {
		email := strings.TrimSpace(userInfo.Email)
		createdBy = &email
	}

	var created StoredSubject
	err := db.QueryRow(`
INSERT INTO shared.nats_subjects (subject, description, payload_format, created_by)
VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4)
RETURNING id, subject, description, payload_format, is_active, created_by, created_at, updated_at
`,
		req.Subject,
		strings.TrimSpace(req.Description),
		strings.TrimSpace(req.PayloadFormat),
		createdBy,
	).Scan(
		&created.ID,
		&created.Subject,
		&created.Description,
		&created.PayloadFormat,
		&created.IsActive,
		&created.CreatedBy,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		logger.Error("failed to create nats subject", "subject", req.Subject, "error", err)
		if strings.Contains(err.Error(), "42P01") || strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"ok":      false,
				"message": "table shared.nats_subjects is missing; run shared migrations and restart server",
				"error":   err.Error(),
			})
		}
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "failed to create subject (may already exist)",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"ok":      true,
		"subject": created,
	})
}

// UpdateStoredSubject handles PUT /api/v1/jetstream/nats-subjects/:id
func UpdateStoredSubject(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_JSH_180")
	defer rc.Close()
	logger := rc.GetLogger()

	db := getSharedDBHandle()
	if db == nil {
		logger.Error("shared db handle is nil")
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "database connection is not initialized",
		})
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "id is required",
		})
	}

	req := UpdateStoredSubjectRequest{}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "invalid request payload",
		})
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "subject is required",
		})
	}

	var updated StoredSubject
	err := db.QueryRow(`
UPDATE shared.nats_subjects
SET subject = $1,
    description = NULLIF($2, ''),
    payload_format = NULLIF($3, ''),
    is_active = $4,
    updated_at = NOW()
WHERE id = $5::bigint
RETURNING id, subject, description, payload_format, is_active, created_by, created_at, updated_at
`,
		req.Subject,
		strings.TrimSpace(req.Description),
		strings.TrimSpace(req.PayloadFormat),
		req.IsActive,
		id,
	).Scan(
		&updated.ID,
		&updated.Subject,
		&updated.Description,
		&updated.PayloadFormat,
		&updated.IsActive,
		&updated.CreatedBy,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			return c.JSON(http.StatusNotFound, map[string]any{
				"ok":      false,
				"message": "subject not found",
			})
		}
		logger.Error("failed to update nats subject", "id", id, "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "failed to update subject",
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"subject": updated,
	})
}

// DeleteStoredSubject handles DELETE /api/v1/jetstream/nats-subjects/:id
func DeleteStoredSubject(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_JSH_190")
	defer rc.Close()
	logger := rc.GetLogger()

	db := getSharedDBHandle()
	if db == nil {
		logger.Error("shared db handle is nil")
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "database connection is not initialized",
		})
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "id is required",
		})
	}

	res, err := db.Exec(`DELETE FROM shared.nats_subjects WHERE id = $1::bigint`, id)
	if err != nil {
		logger.Error("failed to delete nats subject", "id", id, "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "failed to delete subject",
			"error":   err.Error(),
		})
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return c.JSON(http.StatusNotFound, map[string]any{
			"ok":      false,
			"message": "subject not found",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "subject deleted",
	})
}

// PublishEvent handles POST /api/v1/jetstream/events
func PublishEvent(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_JSH_140")
	defer rc.Close()
	logger := rc.GetLogger()

	req := PublishEventRequest{}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "invalid request payload",
		})
	}

	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": "subject is required",
		})
	}
	if err := validateSubjectPayload(req.Subject, req.Payload); err != nil {
		logger.Warn("invalid event payload", "subject", req.Subject, "error", err)
		return c.JSON(http.StatusBadRequest, map[string]any{
			"ok":      false,
			"message": err.Error(),
			"subject": req.Subject,
		})
	}

	connURL := natsURL()
	opts, authMode := natsConnectOptions()
	logger.Info("connecting to nats for jetstream publish", "url", connURL, "auth_mode", authMode)
	nc, err := nats.Connect(connURL, opts...)
	if err != nil {
		logger.Error("failed to connect nats", "url", connURL, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]any{
			"ok":      false,
			"message": "failed to connect to NATS",
			"error":   err.Error(),
		})
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		logger.Error("failed to init jetstream context", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"ok":      false,
			"message": "failed to initialize JetStream context",
		})
	}

	ack, err := js.Publish(req.Subject, []byte(req.Payload))
	if err != nil {
		logger.Error("failed to publish event", "subject", req.Subject, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]any{
			"ok":      false,
			"message": "failed to publish event to JetStream",
			"subject": req.Subject,
			"error":   err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":      true,
		"message": "event published",
		"subject": req.Subject,
		"ack": map[string]any{
			"stream":    ack.Stream,
			"sequence":  ack.Sequence,
			"duplicate": ack.Duplicate,
		},
	})
}
