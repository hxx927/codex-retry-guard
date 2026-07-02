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
				ReasoningEquals []int `json:"reasoning_equals"`
			}{}
			if err := json.Unmarshal(req.Body, &payload); err != nil {
				return pluginapi.ManagementResponse{StatusCode: http.StatusBadRequest, Body: []byte(err.Error())}, nil
			}
			next.ReasoningEquals = pluginconfig.IntList(payload.ReasoningEquals)
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
			return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: body}, nil
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
body {
	margin: 0;
	background: var(--bg);
	color: var(--ink);
	font: 14px/1.55 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}
main {
	max-width: 1120px;
	margin: 0 auto;
	padding: 28px;
}
header {
	display: flex;
	align-items: flex-end;
	justify-content: space-between;
	gap: 16px;
	margin-bottom: 18px;
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
}
.card {
	background: var(--panel);
	border: 1px solid var(--line);
	border-radius: 10px;
	padding: 14px;
	box-shadow: 0 8px 26px rgba(38, 33, 25, .06);
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
		<button id="refresh" type="button">Refresh</button>
	</header>
	<section class="grid" id="metrics"></section>
	<section class="split">
		<div class="card">
			<h2>Recent logs</h2>
			<div id="logs" class="logs"></div>
		</div>
		<div class="card">
			<h2>Last request profile</h2>
			<pre id="profile">Loading...</pre>
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

	function text(value) {
		return value == null ? "" : String(value);
	}

	function card(label, value) {
		return '<div class="card"><div class="label">' + label + '</div><div class="value">' + text(value) + '</div></div>';
	}

	function escapeHTML(value) {
		return text(value).replace(/[&<>"']/g, function (ch) {
			return ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[ch];
		});
	}

	function requestJSON(path) {
		return fetch(path, { credentials: "include", cache: "no-store" }).then(function (resp) {
			if (!resp.ok) {
				return resp.text().then(function (body) {
					throw new Error(resp.status + " " + resp.statusText + (body ? " - " + body : ""));
				});
			}
			return resp.json();
		});
	}

	function render(status, logs) {
		var m = status.metrics || {};
		var cfg = status.config || {};
		metricsEl.innerHTML = [
			card("Inspected", m.inspected_response_count || 0),
			card("Matched", m.matched_response_count || 0),
			card("Blocked", m.blocked_response_count || 0),
			card("Log entries", logs.total_entries || 0)
		].join("");
		summaryEl.innerHTML = cfg.enabled === false ? "Guard is disabled" : '<span class="accent">Guard is enabled</span>';
		profileEl.textContent = JSON.stringify(m.request_profile || {}, null, 2);
		var entries = (logs.entries || []).slice().reverse();
		if (!entries.length) {
			logsEl.innerHTML = '<div class="empty">No plugin log entries yet. Logs appear after the guard inspects or matches traffic.</div>';
			return;
		}
		logsEl.innerHTML = entries.map(function (entry) {
			return '<div class="entry"><div class="meta">#' + escapeHTML(entry.seq) + ' ' + escapeHTML(entry.at) + '</div><pre>' + escapeHTML(entry.message) + '</pre></div>';
		}).join("");
	}

	function refresh() {
		Promise.all([
			requestJSON("/v0/management/plugins/codex-retry-guard/api/status"),
			requestJSON("/v0/management/plugins/codex-retry-guard/api/logs")
		]).then(function (values) {
			render(values[0], values[1]);
		}).catch(function (err) {
			summaryEl.textContent = "Refresh failed";
			logsEl.insertAdjacentHTML("afterbegin", '<div class="error">' + escapeHTML(err.message) + '</div>');
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

	refreshEl.addEventListener("click", refresh);
	initial();
})();
</script>
</body>
</html>`
