package productdrawings

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIProviderDecodesBase64PNG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization header was not forwarded")
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n image")))
	}))
	defer server.Close()

	provider := newOpenAIProvider(server.URL, "test-key")
	got, err := provider.Generate(context.Background(), "openai-image-2.5", "draw a ventilator")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "\x89PNG\r\n\x1a\n") {
		t.Fatalf("decoded image = %q", got)
	}
}

func TestOpenAIProviderDoesNotLeakAPIKeyInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider rejected secret", http.StatusBadRequest)
	}))
	defer server.Close()

	provider := newOpenAIProvider(server.URL, "test-key")
	_, err := provider.Generate(context.Background(), "model", "prompt")
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("error = %v", err)
	}
}
