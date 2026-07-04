# Codex Retry Guard

Codex Retry Guard is a CPA dynamic plugin that ports the core `reasoning_tokens` guard from `codex-retry-gateway` into CLIProxyAPI/CPA.

It inspects normal and streaming model responses, watches for suspicious `reasoning_tokens` values such as `516`, `1034`, and `1552`, and can retry or block those responses inside CPA. It also exposes a small management page for status, recent logs, and runtime counters.

## Features

- Request interceptor that can add `stream_options.include_usage=true` for matching streaming requests.
- Non-streaming response interceptor based on `usage.output_tokens_details.reasoning_tokens` and `usage.completion_tokens_details.reasoning_tokens`.
- Streaming response interceptor for SSE usage chunks.
- Optional model allow-list through `plugins.configs.codex-retry-guard.models`.
- Internal retry through CPA host model callbacks.
- Management API and resource page for live status.

## CPA config example

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    codex-retry-guard:
      enabled: true
      priority: 1
      models: []
      auto_include_stream_usage: true
      reasoning_equals: [516, 1034, 1552]
      intercept_streaming: true
      intercept_non_streaming: true
      guard_retry_attempts: 3
      non_stream_status_code: 502
      stream_action: strict_502
      log_match: true
```

Leave `models` empty to inspect all models, or set exact model names to inspect only those models.

## Management page authentication

The management page calls CPA management APIs from a plugin iframe. Since v0.1.7, the page reads the management key from both the legacy `managementKey` storage entry and the current CPAMC `cli-proxy-auth.state.managementKey` storage entry.

If the CPAMC login does not persist the management key, the key may exist only in the parent page runtime memory. A plugin iframe cannot read that memory by itself. In that case the page needs a CPAMC-side bridge, or the user must sign in with key persistence enabled. This is a browser isolation limit, not a guard runtime issue.

## Build

```bash
go test ./... -count=1
./scripts/build-release.sh 0.1.0
```

The release script produces the CPA plugin store assets required by the official plugin documentation:

```text
dist/codex-retry-guard_0.1.0_linux_amd64.zip
dist/checksums.txt
```

The zip root directly contains `codex-retry-guard.so`.

## Plugin store metadata

A third-party registry entry can point to this repository:

```json
{
  "schema_version": 1,
  "plugins": [
    {
      "id": "codex-retry-guard",
      "name": "Codex Retry Guard",
      "description": "Inspect Codex responses and retry or block suspicious reasoning_tokens values.",
      "author": "hxx927",
      "version": "0.1.0",
      "repository": "https://github.com/hxx927/codex-retry-guard",
      "license": "MIT",
      "tags": ["codex", "guard", "reasoning", "retry"]
    }
  ]
}
```

The real install version is resolved from the latest GitHub release tag, as required by CPA.
