package web_test

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

const defaultHome3URL = "http://127.0.0.1:5173/home3"

func TestHome3KnowledgeSystemOpensKnowledgeInNewTab(t *testing.T) {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping CDP GUI test in short mode")
	}

	home3URL := os.Getenv("CHENWEB_HOME3_URL")
	if strings.TrimSpace(home3URL) == "" {
		home3URL = defaultHome3URL
	}
	if !isHTTPReachable(home3URL) {
		t.Skipf("home3 UI is not reachable at %s", home3URL)
	}

	chromeExec, ok := findChromeExecutable()
	if !ok {
		t.Skip("Google Chrome/Chromium executable not found")
	}

	controlURL := launcher.New().
		Bin(chromeExec).
		Headless(true).
		NoSandbox(true).
		MustLaunch()

	browser := rod.New().ControlURL(controlURL).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(home3URL)
	defer page.MustClose()

	page.Timeout(20 * time.Second).MustWaitLoad()
	page.MustElement(`button[aria-label="Knowledge System"]`).MustWaitVisible()
	page.MustEval(`() => {
		window.__chenwebOpenCalls = [];
		window.open = (...args) => {
			window.__chenwebOpenCalls.push(args);
			return null;
		};
	}`)
	time.Sleep(500 * time.Millisecond)
	page.MustEval(`() => {
		const button = document.querySelector('button[aria-label="Knowledge System"]');
		if (!button) throw new Error('Knowledge System button not found');
		button.click();
	}`)

	openCall := page.Timeout(5 * time.Second).MustEval(`() => window.__chenwebOpenCalls[window.__chenwebOpenCalls.length - 1]`)
	if len(openCall.Arr()) < 2 {
		t.Fatalf("window.open result = %#v, want at least url and target", openCall)
	}

	rawURL := openCall.Get("0").Str()
	target := openCall.Get("1").Str()

	openedURL, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse opened url %q: %v", rawURL, err)
	}

	if openedURL.Path != "/home3/knowledge" {
		t.Fatalf("opened path = %q, want %q", openedURL.Path, "/home3/knowledge")
	}

	if target != "_blank" {
		t.Fatalf("window.open target = %q, want %q", target, "_blank")
	}
}

func isHTTPReachable(target string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func findChromeExecutable() (string, bool) {
	if path := strings.TrimSpace(os.Getenv("CHENWEB_CHROME_BIN")); path != "" {
		return path, true
	}

	candidates := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"google-chrome",
		"chromium",
		"chromium-browser",
		"chrome",
	}

	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, "/") {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, true
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, true
		}
	}

	return "", false
}
