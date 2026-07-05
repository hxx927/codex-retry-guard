package management

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	pluginconfig "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/config"
	pluginruntime "github.com/router-for-me/CLIProxyAPI/v7/plugins/codex-retry-guard/internal/runtime"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type Registration struct {
	Routes    []pluginapi.ManagementRoute
	Resources []pluginapi.ResourceRoute
}

type handlerFunc func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error)

func (f handlerFunc) HandleManagement(_ context.Context, req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	return f(req)
}

func Register(state *pluginruntime.State) Registration {
	status := pluginapi.ManagementRoute{
		Method: http.MethodGet,
		Path:   "/plugins/codex-retry-guard/api/status",
		Handler: handlerFunc(func(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			body, _ := json.Marshal(map[string]any{
				"config":  state.Config(),
				"metrics": state.Metrics().Snapshot(),
			})
			return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}, Body: body}, nil
		}),
	}
	logs := pluginapi.ManagementRoute{
		Method: http.MethodGet,
		Path:   "/plugins/codex-retry-guard/api/logs",
		Handler: handlerFunc(func(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			snapshot := state.Metrics().Snapshot()
			latestSeq := int64(0)
			if n := len(snapshot.Logs); n > 0 {
				latestSeq = snapshot.Logs[n-1].Seq
			}
			body, _ := json.Marshal(map[string]any{
				"entries":       snapshot.Logs,
				"total_entries": len(snapshot.Logs),
				"latest_seq":    latestSeq,
			})
			return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}, Body: body}, nil
		}),
	}
	configGet := pluginapi.ManagementRoute{
		Method: http.MethodGet,
		Path:   "/plugins/codex-retry-guard/config",
		Handler: handlerFunc(func(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			body, _ := json.Marshal(state.Config())
			return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}, Body: body}, nil
		}),
	}
	configPost := pluginapi.ManagementRoute{
		Method: http.MethodPost,
		Path:   "/plugins/codex-retry-guard/config",
		Handler: handlerFunc(func(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			next := state.Config()
			payload := struct {
				ReasoningEquals    []int  `json:"reasoning_equals"`
				ReasoningMatchMode string `json:"reasoning_match_mode"`
			}{}
			if err := json.Unmarshal(req.Body, &payload); err != nil {
				return pluginapi.ManagementResponse{StatusCode: http.StatusBadRequest, Body: []byte(err.Error())}, nil
			}
			next.ReasoningEquals = pluginconfig.IntList(payload.ReasoningEquals)
			if payload.ReasoningMatchMode != "" {
				next.ReasoningMatchMode = payload.ReasoningMatchMode
			}
			if err := state.Reconfigure(next); err != nil {
				return pluginapi.ManagementResponse{StatusCode: http.StatusBadRequest, Body: []byte(err.Error())}, nil
			}
			body, _ := json.Marshal(state.Config())
			return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}, Body: body}, nil
		}),
	}
	resource := pluginapi.ResourceRoute{
		Path:        "/status",
		Menu:        "Codex Retry Guard",
		Description: "Shows retry guard status.",
		Handler: handlerFunc(func(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			body := renderStatusPage(state)
			return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}, "Cache-Control": []string{"no-store"}}, Body: body}, nil
		}),
	}
	return Registration{Routes: []pluginapi.ManagementRoute{status, logs, configGet, configPost}, Resources: []pluginapi.ResourceRoute{resource}}
}

var _ = pluginconfig.Config{}

func renderStatusPage(state *pluginruntime.State) []byte {
	snapshot := state.Metrics().Snapshot()
	logs := snapshot.Logs
	if logs == nil {
		logs = []pluginruntime.LogEntry{}
	}
	latestSeq := int64(0)
	if n := len(logs); n > 0 {
		latestSeq = logs[n-1].Seq
	}
	payload, _ := json.Marshal(map[string]any{
		"status": map[string]any{
			"config":  state.Config(),
			"metrics": snapshot,
		},
		"logs": map[string]any{
			"entries":       logs,
			"total_entries": len(logs),
			"latest_seq":    latestSeq,
		},
	})
	return []byte(strings.Replace(statusPageHTML, "__INITIAL_PAYLOAD__", string(payload), 1))
}

const statusPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Codex Retry Guard</title>
<style>
:root {
	color-scheme: light;
	--bg: #f7f5ef;
	--panel: #fffdf8;
	--ink: #25231f;
	--muted: #736f66;
	--line: #e5dfd2;
	--accent: #2f7d62;
	--warn: #b65d31;
}
* { box-sizing: border-box; }
html, body {
	height: 100%;
	overflow: hidden;
}
body {
	margin: 0;
	background: var(--bg);
	color: var(--ink);
	font: 14px/1.55 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
main {
	height: 100vh;
	max-width: 1120px;
	margin: 0 auto;
	padding: 28px;
	display: flex;
	flex-direction: column;
	min-height: 0;
}
header {
	display: flex;
	align-items: flex-end;
	justify-content: space-between;
	gap: 16px;
	margin-bottom: 18px;
	flex: 0 0 auto;
}
h1 {
	margin: 0;
	font-size: 24px;
	line-height: 1.2;
}
.sub {
	color: var(--muted);
	margin-top: 4px;
}
button {
	border: 1px solid var(--line);
	background: var(--panel);
	color: var(--ink);
	border-radius: 8px;
	padding: 8px 12px;
	cursor: pointer;
}
button:hover { border-color: #cfc5b4; }
.grid {
	display: grid;
	grid-template-columns: repeat(4, minmax(0, 1fr));
	gap: 12px;
	margin-bottom: 12px;
	flex: 0 0 auto;
}
.card {
	background: var(--panel);
	border: 1px solid var(--line);
	border-radius: 10px;
	padding: 14px;
	box-shadow: 0 8px 26px rgba(38, 33, 25, .06);
}
.scroll-panel {
	display: flex;
	flex-direction: column;
	min-height: 0;
	overflow: hidden;
}
.scroll-body {
	flex: 1 1 auto;
	min-height: 0;
	overflow: auto;
	padding-right: 6px;
}
.label {
	color: var(--muted);
	font-size: 12px;
	text-transform: uppercase;
	letter-spacing: .04em;
}
.value {
	font-size: 26px;
	font-weight: 750;
	margin-top: 4px;
}
.split {
	display: grid;
	grid-template-columns: minmax(0, 1fr) minmax(320px, 420px);
	gap: 12px;
	flex: 1 1 auto;
	min-height: 0;
	align-items: stretch;
}
h2 {
	font-size: 15px;
	margin: 0 0 10px;
}
pre {
	margin: 0;
	white-space: pre-wrap;
	word-break: break-word;
	font: 12px/1.5 ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
	color: #34302a;
}
.logs {
	display: grid;
	gap: 8px;
}
#profile {
	white-space: pre;
	word-break: normal;
}
.entry {
	border: 1px solid var(--line);
	border-radius: 8px;
	padding: 10px;
	background: #fffaf1;
}
.entry .meta {
	color: var(--muted);
	font-size: 12px;
	margin-bottom: 4px;
}
.section-head {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 12px;
	margin-bottom: 14px;
}
.section-head h2 { margin: 0; }
.refresh-controls {
	display: inline-flex;
	align-items: center;
	justify-content: flex-end;
	gap: 10px;
	flex-wrap: wrap;
}
.limit-control {
	display: inline-flex;
	align-items: center;
	gap: 8px;
	color: var(--muted);
	font-size: 13px;
}
.limit-control input {
	width: 72px;
	border: 1px solid var(--line);
	border-radius: 9px;
	padding: 7px 8px;
	background: rgba(255,255,255,.78);
}
.empty, .error {
	border: 1px dashed var(--line);
	border-radius: 10px;
	padding: 18px;
	color: var(--muted);
	background: rgba(255,255,255,.48);
}
.error { color: var(--warn); }
.accent { color: var(--accent); }
@media (max-width: 860px) {
	main { padding: 18px; }
	header { align-items: flex-start; flex-direction: column; }
	.grid, .split { grid-template-columns: 1fr; }
	.section-head { align-items: flex-start; flex-direction: column; }
	.refresh-controls { justify-content: flex-start; }
}
</style>
</head>
<body>
<main>
	<header>
		<div>
			<h1>Codex Retry Guard</h1>
			<div class="sub" id="summary">Loading plugin status...</div>
		</div>
	</header>
	<section class="grid" id="metrics"></section>
	<section class="split">
		<div class="card scroll-panel">
			<div class="section-head">
				<h2>Recent logs</h2>
				<div class="refresh-controls">
					<label class="limit-control"><input id="auto-refresh" type="checkbox" checked> Auto refresh</label>
					<label class="limit-control">Show <input id="log-limit" type="number" min="1" max="100" step="1" value="100"> rows</label>
					<button id="refresh" type="button">Refresh</button>
				</div>
			</div>
			<div class="scroll-body"><div id="logs" class="logs"></div></div>
		</div>
		<div class="card scroll-panel">
			<h2>Last request profile</h2>
			<div class="scroll-body"><pre id="profile">Loading...</pre></div>
		</div>
	</section>
</main>
<script type="application/json" id="initial-data">__INITIAL_PAYLOAD__</script>
<script>
(function () {
	var metricsEl = document.getElementById("metrics");
	var logsEl = document.getElementById("logs");
	var profileEl = document.getElementById("profile");
	var summaryEl = document.getElementById("summary");
	var refreshEl = document.getElementById("refresh");
	var autoRefreshEl = document.getElementById("auto-refresh");
	var logLimitEl = document.getElementById("log-limit");
	var latestStatus = null;
	var latestLogs = null;
	var latestRenderedAt = 0;
	var latestActivityRate = null;
	var refreshing = false;

	function text(value) {
		return value == null ? "" : String(value);
	}

	function card(label, value) {
		return '<div class="card"><div class="label">' + label + '</div><div class="value">' + text(value) + '</div></div>';
	}

	function number(value) {
		var n = Number(value || 0);
		return Number.isFinite(n) ? n : 0;
	}

	function formatPercent(numerator, denominator) {
		numerator = number(numerator);
		denominator = number(denominator);
		if (denominator <= 0) return "0%";
		return (numerator * 100 / denominator).toFixed(2).replace(/\.?0+$/, "") + "%";
	}

	function formatActivity(rate) {
		if (rate == null) return "Watching";
		if (rate <= 0) return "Idle";
		var formatted = rate >= 10 ? rate.toFixed(0) : rate.toFixed(1).replace(/\.0$/, "");
		if (rate >= 100) return "High " + formatted + "/s";
		if (rate >= 20) return "Active " + formatted + "/s";
		return "Normal " + formatted + "/s";
	}

	function escapeHTML(value) {
		return text(value).replace(/[&<>"']/g, function (ch) {
			return ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[ch];
		});
	}

	function decodeStorageValue(raw) {
		var prefix = "enc::v1::";
		var value = raw;
		if (raw && raw.indexOf(prefix) === 0 && window.TextEncoder && window.TextDecoder && window.atob) {
			try {
				var keyText = "cli-proxy-api-webui::secure-storage|" + window.location.host + "|" + navigator.userAgent;
				var key = new TextEncoder().encode(keyText);
				var binary = window.atob(raw.slice(prefix.length));
				var bytes = new Uint8Array(binary.length);
				for (var i = 0; i < binary.length; i++) {
					bytes[i] = binary.charCodeAt(i) ^ key[i % key.length];
				}
				value = new TextDecoder().decode(bytes);
			} catch (err) {
				value = raw;
			}
		}
		try {
			return JSON.parse(value);
		} catch (err) {
			return value;
		}
	}

	function storedValue(name) {
		if (!window.localStorage) return null;
		return decodeStorageValue(window.localStorage.getItem(name));
	}

	function managementKey() {
		try {
			var key = storedValue("managementKey");
			if (typeof key === "string" && key.trim()) return key.trim();

			var auth = storedValue("cli-proxy-auth");
			if (auth && typeof auth === "object") {
				var state = auth.state && typeof auth.state === "object" ? auth.state : auth;
				key = state && state.managementKey;
				if (typeof key === "string" && key.trim()) return key.trim();
			}
		} catch (err) {
			return "";
		}
		return "";
	}

	function logLimit() {
		var n = parseInt(logLimitEl && logLimitEl.value, 10);
		if (!Number.isFinite(n) || n < 1) return 100;
		return Math.min(n, 100);
	}

	function requestJSON(path) {
		var key = managementKey();
		var headers = key ? { Authorization: "Bearer " + key, "X-Management-Key": key } : {};
		return fetch(path, { credentials: "include", cache: "no-store", headers: headers }).then(function (resp) {
			if (!resp.ok) {
				return resp.text().then(function (body) {
					throw new Error(resp.status + " " + resp.statusText + (body ? " - " + body : ""));
				});
			}
			return resp.json();
		});
	}

	function render(status, logs) {
		var previousStatus = latestStatus;
		var previousRenderedAt = latestRenderedAt;
		var renderedAt = Date.now();
		if (previousStatus && previousRenderedAt > 0) {
			var prevMetrics = previousStatus.metrics || {};
			var nextMetrics = status.metrics || {};
			var inspectedDelta = Math.max(0, number(nextMetrics.inspected_response_count) - number(prevMetrics.inspected_response_count));
			var seconds = Math.max(1, (renderedAt - previousRenderedAt) / 1000);
			latestActivityRate = inspectedDelta / seconds;
		}
		latestStatus = status;
		latestLogs = logs;
		latestRenderedAt = renderedAt;
		var m = status.metrics || {};
		var cfg = status.config || {};
		metricsEl.innerHTML = [
			card("Inspected", m.inspected_response_count || 0),
			card("Matched", m.matched_response_count || 0),
			card("Blocked", m.blocked_response_count || 0),
			card("Match rate", formatPercent(m.matched_response_count, m.inspected_response_count)),
			card("Block rate", formatPercent(m.blocked_response_count, m.inspected_response_count)),
			card("Activity", formatActivity(latestActivityRate)),
			card("Log entries", logs.total_entries || 0)
		].join("");
		summaryEl.innerHTML = cfg.enabled === false ? "Guard is disabled" : '<span class="accent">Guard is enabled</span>';
		profileEl.textContent = JSON.stringify(m.request_profile || {}, null, 2);
		var allEntries = (logs.entries || []).slice().reverse();
		var limit = logLimit();
		var entries = allEntries.slice(0, limit);
		if (!entries.length) {
			logsEl.innerHTML = '<div class="empty">No plugin log entries yet. Logs appear after the guard inspects or matches traffic.</div>';
			return;
		}
		var limited = allEntries.length > entries.length ? '<div class="empty">Showing latest ' + escapeHTML(entries.length) + ' of ' + escapeHTML(allEntries.length) + ' loaded log entries. Raise the row limit to see more.</div>' : '';
		logsEl.innerHTML = limited + entries.map(function (entry) {
			return '<div class="entry"><div class="meta">#' + escapeHTML(entry.seq) + ' ' + escapeHTML(entry.at) + '</div><pre>' + escapeHTML(entry.message) + '</pre></div>';
		}).join("");
	}

	function refresh(silent) {
		if (refreshing) return;
		refreshing = true;
		if (refreshEl) refreshEl.disabled = true;
		Promise.all([
			requestJSON("/v0/management/plugins/codex-retry-guard/api/status"),
			requestJSON("/v0/management/plugins/codex-retry-guard/api/logs")
		]).then(function (values) {
			render(values[0], values[1]);
		}).catch(function (err) {
			summaryEl.textContent = "Refresh failed";
			if (!silent) logsEl.insertAdjacentHTML("afterbegin", '<div class="error">' + escapeHTML(err.message) + '</div>');
		}).finally(function () {
			refreshing = false;
			if (refreshEl) refreshEl.disabled = false;
		});
	}

	function initial() {
		var raw = document.getElementById("initial-data");
		if (!raw) {
			refresh();
			return;
		}
		try {
			var payload = JSON.parse(raw.textContent || "{}");
			render(payload.status || {}, payload.logs || {});
		} catch (err) {
			refresh();
		}
	}

	refreshEl.addEventListener("click", function () { refresh(false); });
	window.setInterval(function () {
		if (autoRefreshEl && autoRefreshEl.checked) refresh(true);
	}, 10000);
	if (logLimitEl) {
		logLimitEl.addEventListener("change", function () {
			if (latestStatus && latestLogs) render(latestStatus, latestLogs);
		});
	}
	initial();
})();
</script>
</body>
</html>`
