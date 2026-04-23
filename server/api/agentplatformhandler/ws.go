package agentplatformhandler

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// -----------------------------------------------------------------------------
// WebSocket handler (M2)
//
// GET /ws/agent-platform?workspace=<slug>
//
// Upgrades the request to a WebSocket, subscribes to the in-process hub for
// the named workspace, and streams WSEvent JSON frames until either side
// disconnects.
//
// Auth: relies on authmiddleware.AuthMiddleware being wired on the /ws
// route group in routes.go. Membership is re-checked here via
// resolveWorkspace so a caller can't subscribe to a workspace they aren't
// in.
// -----------------------------------------------------------------------------

const (
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10 // 54s
	wsWriteWait  = 10 * time.Second
	wsMaxMsgSize = 1024 // bytes — clients don't need to send much
)

// wsUpgrader is shared by every WS handler call. CheckOrigin compares the
// Origin header against VITE_DEV_ONLY_URL in dev, and falls back to
// same-host comparison in prod.
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     wsCheckOrigin,
}

func wsCheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Non-browser clients (server-side probes) have no Origin. Allow.
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	// Dev: explicit allowlist via VITE_DEV_ONLY_URL (the same var routes.go
	// uses for the frontend proxy).
	if dev := os.Getenv("VITE_DEV_ONLY_URL"); dev != "" {
		if devURL, err := url.Parse(dev); err == nil && devURL.Host == u.Host {
			return true
		}
	}
	// Prod: same host as the request.
	return u.Host == r.Host
}

// HandleAgentPlatformWS upgrades the request and streams hub events.
// Errors before upgrade are written as JSON (so the handshake surfaces them
// to the client's WebSocket constructor).
func HandleAgentPlatformWS(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_WS")
	if !ok {
		return nil
	}
	slug := c.QueryParam("workspace")
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, slug, false)
	if !ok {
		return nil
	}
	hub := InitHub()

	ws, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		// Upgrade failed — response is already written by the upgrader.
		return nil
	}
	defer ws.Close()

	ch, unsub := hub.Subscribe(wid)
	defer unsub()

	// Keepalive wiring.
	ws.SetReadLimit(wsMaxMsgSize)
	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error {
		_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Read pump: we don't expect client frames, but we must drain the
	// connection for pongs + close frames. Any error ends the session.
	go func() {
		defer cancel()
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Write pump: fan hub events out as JSON, plus periodic pings.
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	// First frame: a hello so clients can confirm delivery.
	_ = writeJSONFrame(ws, WSEvent{Type: "hello", Payload: map[string]any{"workspace": slug}})

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-ch:
			if !ok {
				// Hub dropped us (overflow or shutdown). Politely close.
				_ = ws.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "backpressure"),
					time.Now().Add(wsWriteWait),
				)
				return nil
			}
			if err := writeJSONFrame(ws, evt); err != nil {
				return nil
			}
		case <-ticker.C:
			_ = ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return nil
			}
		}
	}
}

func writeJSONFrame(ws *websocket.Conn, evt WSEvent) error {
	_ = ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
	return ws.WriteJSON(evt)
}
