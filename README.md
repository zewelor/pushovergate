# pushovergate

Minimal, safe Go service for forwarding notifications to Pushover from a private Kubernetes cluster.

## Configuration

`pushovergate` requires three environment variables:

| Variable | Required | Description |
| --- | --- | --- |
| `API_TOKEN` | Yes | Bearer token for client authentication, validated at startup as exactly 64 hexadecimal characters. |
| `PUSHOVER_APP_TOKEN` | Yes | Pushover application API token. |
| `PUSHOVER_USER_KEY` | Yes | Pushover user or group key. |

The service listens on hardcoded port `:8080` and forwards requests to `https://api.pushover.net/1/messages.json`.

## HTTP API

### `POST /messages`

Synchronously forwards a message to Pushover.

- **Authentication:** `Authorization: Bearer <API_TOKEN>` (compared in constant time using SHA-256 digests).
- **Request Body:** Single JSON object with a required `message` field (up to 8 KiB body limit; `Content-Type` is neither required nor inspected; unknown fields and trailing data are rejected; `message` must contain at least 1 non-whitespace rune and at most 1024 Unicode characters).
  ```json
  {"message": "Deployment completed successfully"}
  ```
- **Responses:**
  - `204 No Content`: Message accepted and queued by Pushover (`status: 1`).
  - `400 Bad Request`: Malformed JSON, unknown fields, trailing data, or missing/empty/too-long message.
  - `401 Unauthorized`: Missing or invalid Bearer token.
  - `413 Request Entity Too Large`: Request body exceeds 8 KiB.
  - `502 Bad Gateway`: Pushover returned an error, non-200 status, or was unreachable.

All error responses use plain text bodies.

### `GET /healthz`

Unauthenticated operational liveness probe returning `204 No Content`.

## Usage

```bash
export API_TOKEN="$(openssl rand -hex 32)"

docker run -d \
  -p 8080:8080 \
  -e API_TOKEN="$API_TOKEN" \
  -e PUSHOVER_APP_TOKEN="your_pushover_app_token" \
  -e PUSHOVER_USER_KEY="your_pushover_user_key" \
  ghcr.io/zewelor/pushovergate:latest
```

Send a notification:

```bash
curl -i -X POST \
  -H "Authorization: Bearer $API_TOKEN" \
  -d '{"message":"Deployment completed successfully"}' \
  http://localhost:8080/messages
```

## Development

Run repository checks using `just`:

```bash
# Run tests in Docker test stage
just test

# Run full CI quality gate (tests and production build)
just ci

# Inspect files included in Docker build context
just show_dockerignore
```

## Scope

Kubernetes manifests, TLS, queues, retries, persistence, UI, metrics, tracing, and optional Pushover fields are explicitly out of scope.
