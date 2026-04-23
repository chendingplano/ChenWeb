// WebSocket client for the Agent Platform realtime stream.
//
// Server: ChenWeb/server/api/agentplatformhandler/ws.go
// Endpoint: /ws/agent-platform?workspace=<slug>  (same-origin cookie auth)
//
// Frame shape: { type: string, payload: any }  — matches hub.go WSEvent.
//
// Auto-reconnects with capped exponential backoff while the browser tab
// stays mounted. The returned disposer stops the socket and halts further
// reconnect attempts.

export type WSFrame = {
	type: string;
	payload: unknown;
};

export type WSStatus = 'connecting' | 'open' | 'closed';

export type WSOptions = {
	onFrame: (frame: WSFrame) => void;
	onStatus?: (status: WSStatus) => void;
	// Max backoff between reconnects, in ms. Default 15s.
	maxBackoffMs?: number;
};

export function connectAgentPlatformWS(slug: string, opts: WSOptions): () => void {
	let ws: WebSocket | null = null;
	let stopped = false;
	let attempt = 0;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

	const maxBackoffMs = opts.maxBackoffMs ?? 15_000;
	const setStatus = opts.onStatus ?? (() => {});

	function scheduleReconnect() {
		if (stopped) return;
		attempt += 1;
		// 500ms, 1s, 2s, 4s, 8s, …, capped at maxBackoffMs, +20% jitter.
		const base = Math.min(maxBackoffMs, 500 * Math.pow(2, attempt - 1));
		const jitter = base * 0.2 * Math.random();
		const delay = Math.floor(base + jitter);
		reconnectTimer = setTimeout(open, delay);
	}

	function open() {
		if (stopped) return;
		setStatus('connecting');
		const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${proto}//${window.location.host}/ws/agent-platform?workspace=${encodeURIComponent(slug)}`;
		let sock: WebSocket;
		try {
			sock = new WebSocket(url);
		} catch {
			scheduleReconnect();
			return;
		}
		ws = sock;

		sock.addEventListener('open', () => {
			attempt = 0;
			setStatus('open');
		});
		sock.addEventListener('message', (ev) => {
			if (typeof ev.data !== 'string') return;
			try {
				const frame = JSON.parse(ev.data) as WSFrame;
				if (frame && typeof frame.type === 'string') {
					opts.onFrame(frame);
				}
			} catch {
				/* ignore malformed frames */
			}
		});
		sock.addEventListener('close', () => {
			if (ws === sock) ws = null;
			setStatus('closed');
			scheduleReconnect();
		});
		sock.addEventListener('error', () => {
			// The close event will also fire; reconnect happens there.
			try {
				sock.close();
			} catch {
				/* ignore */
			}
		});
	}

	open();

	return function dispose() {
		stopped = true;
		if (reconnectTimer !== null) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
		if (ws) {
			try {
				ws.close();
			} catch {
				/* ignore */
			}
			ws = null;
		}
		setStatus('closed');
	};
}
