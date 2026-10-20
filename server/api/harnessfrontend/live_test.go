//go:build live

// Live end-to-end check through a running gateway and the real harness:
//
//	HARNESS_GATEWAY_URL=http://127.0.0.1:4318 HARNESS_GATEWAY_SECRET=... go test -tags live -run TestLive -v ./api/harnessfrontend/
package harnessfrontend

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type liveEvent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	SessionID string          `json:"sessionId"`
	RequestID string          `json:"requestId"`
	ToolName  string          `json:"toolName"`
	Name      string          `json:"name"`
	Status    string          `json:"status"`
	Input     json.RawMessage `json:"input"`
}

func TestLiveTurnsThroughChenWeb(t *testing.T) {
	url, secret := os.Getenv("HARNESS_GATEWAY_URL"), os.Getenv("HARNESS_GATEWAY_SECRET")
	if url == "" || secret == "" {
		t.Skip("HARNESS_GATEWAY_URL and HARNESS_GATEWAY_SECRET are required")
	}
	withUser(t, &ApiTypes.UserInfo{UserId: "admin", Roles: []string{"admin"}})
	registry := NewRegistry()
	_ = registry.Add(NewHost("claude-code", url, secret, nil))
	e := echo.New()
	RegisterRoutes(e.Group("/api/v1/harness-frontend"), NewHandler(registry))
	server := httptest.NewServer(e)
	defer server.Close()
	base := server.URL + "/api/v1/harness-frontend/claude-code"
	target := "/tmp/chenweb-harness-e2e.txt"
	_ = os.Remove(target)

	turn := func(body string) (string, []liveEvent) {
		response, err := http.Post(base+"/turns", "application/json", strings.NewReader(body))
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("turn: status=%v err=%v", response, err)
		}
		defer response.Body.Close()
		var answer strings.Builder
		var events []liveEvent
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), maxEventBytes)
		sessionID := ""
		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			var event liveEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				t.Fatalf("bad event %s", data)
			}
			events = append(events, event)
			switch event.Type {
			case "session":
				sessionID = event.SessionID
			case "text":
				answer.WriteString(event.Text)
			case "tool_use":
				t.Logf("tool_use %s", event.Name)
			case "permission_request":
				t.Logf("permission_request %s %s -> allow", event.ToolName, event.Input)
				decision, _ := http.Post(base+"/sessions/"+sessionID+"/permissions/"+event.RequestID, "application/json", strings.NewReader(`{"behavior":"allow"}`))
				if decision == nil || decision.StatusCode != http.StatusOK {
					t.Fatalf("decision failed: %v", decision)
				}
			case "result":
				t.Logf("result %s", event.Status)
			}
		}
		return answer.String(), events
	}

	answer, events := turn(`{"message":"Remember the codeword OTTER-7. Then use the Write tool to create ` + target + ` containing the single word hello. Reply in one short line."}`)
	sessionID := ""
	for _, event := range events {
		if event.Type == "session" {
			sessionID = event.SessionID
		}
	}
	t.Logf("turn 1 answer: %s", answer)
	if content, err := os.ReadFile(target); err != nil || strings.TrimSpace(string(content)) != "hello" {
		t.Fatalf("file not written: %q err=%v", content, err)
	}
	answer, _ = turn(`{"session_id":"` + sessionID + `","message":"What codeword did I give you? Reply with the codeword only."}`)
	t.Logf("turn 2 answer: %s", answer)
	if !strings.Contains(answer, "OTTER-7") {
		t.Fatalf("harness did not keep the conversation: %q", answer)
	}
	_ = os.Remove(target)
}
