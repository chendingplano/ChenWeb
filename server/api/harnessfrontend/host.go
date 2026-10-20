// Package harnessfrontend makes ChenWeb a front end to external agent
// harnesses, the way an IDE is a front end to Claude Code. The harness owns
// the system prompt, agent loop, tools, memory and conversation history;
// ChenWeb forwards the user's messages and decisions and relays what the
// harness streams back. It stores nothing about the conversation.
//
// Each harness runs behind a private gateway that speaks harness front-end
// protocol v1 (see gateways/claude-code). A Host is a client for one gateway.
package harnessfrontend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Host is a protocol-v1 client for one harness gateway.
type Host struct {
	name    string
	baseURL string
	secret  string
	client  *http.Client
}

func NewHost(name, rawURL, secret string, client *http.Client) *Host {
	if client == nil {
		// No overall timeout: a turn streams for as long as the harness works.
		client = &http.Client{}
	}
	return &Host{name: name, baseURL: strings.TrimRight(strings.TrimSpace(rawURL), "/"), secret: secret, client: client}
}

func (h *Host) Name() string { return h.name }

// TurnRequest is one user message for a new or existing harness session.
type TurnRequest struct {
	SessionID      string `json:"sessionId,omitempty"`
	Message        string `json:"message"`
	PermissionMode string `json:"permissionMode,omitempty"`
}

// Decision answers a permission request. UpdatedInput carries changed tool
// input, such as the answers to a question the harness asked.
type Decision struct {
	Behavior     string          `json:"behavior"`
	UpdatedInput json.RawMessage `json:"updatedInput,omitempty"`
	Message      string          `json:"message,omitempty"`
}

// Response is a gateway reply relayed as is.
type Response struct {
	Status int
	Body   []byte
}

func (h *Host) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	if h == nil || h.baseURL == "" || h.secret == "" {
		return nil, errors.New("harness gateway is not configured")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+h.secret)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return h.client.Do(request)
}

// maxReplyBytes bounds non-streaming replies; session history can be large.
const maxReplyBytes = 32 << 20

func (h *Host) relay(ctx context.Context, method, path string, body any) (Response, error) {
	response, err := h.do(ctx, method, path, body)
	if err != nil {
		return Response{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReplyBytes+1))
	if err != nil {
		return Response{}, err
	}
	if len(data) > maxReplyBytes {
		return Response{}, fmt.Errorf("harness %s reply is too large", h.name)
	}
	return Response{Status: response.StatusCode, Body: data}, nil
}

func (h *Host) ListSessions(ctx context.Context, limit, offset int) (Response, error) {
	return h.relay(ctx, http.MethodGet, fmt.Sprintf("/v1/sessions?limit=%d&offset=%d", limit, offset), nil)
}

func (h *Host) SessionMessages(ctx context.Context, sessionID string, limit, offset int) (Response, error) {
	return h.relay(ctx, http.MethodGet, fmt.Sprintf("/v1/sessions/%s/messages?limit=%d&offset=%d", url.PathEscape(sessionID), limit, offset), nil)
}

func (h *Host) Interrupt(ctx context.Context, sessionID string) (Response, error) {
	return h.relay(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(sessionID)+"/interrupt", nil)
}

func (h *Host) Decide(ctx context.Context, sessionID, requestID string, decision Decision) (Response, error) {
	return h.relay(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(sessionID)+"/permissions/"+url.PathEscape(requestID), decision)
}

// Turn starts a turn. On success the caller reads NDJSON events from the
// returned body until it ends; cancelling ctx interrupts the turn.
func (h *Host) Turn(ctx context.Context, turn TurnRequest) (io.ReadCloser, Response, error) {
	response, err := h.do(ctx, http.MethodPost, "/v1/turns", turn)
	if err != nil {
		return nil, Response{}, err
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return nil, Response{Status: response.StatusCode, Body: data}, nil
	}
	return response.Body, Response{Status: http.StatusOK}, nil
}

// Registry holds the configured harness gateways by name.
type Registry struct{ hosts map[string]*Host }

func NewRegistry() *Registry { return &Registry{hosts: make(map[string]*Host)} }

func (r *Registry) Add(host *Host) error {
	if host == nil || host.name == "" {
		return errors.New("harness host needs a name")
	}
	if _, exists := r.hosts[host.name]; exists {
		return fmt.Errorf("harness %q is already registered", host.name)
	}
	r.hosts[host.name] = host
	return nil
}

func (r *Registry) Get(name string) (*Host, bool) {
	host, ok := r.hosts[name]
	return host, ok
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.hosts))
	for name := range r.hosts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
