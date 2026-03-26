# Gocket

A Pusher-compatible WebSocket server written in Go. Drop-in replacement for [Soketi](https://soketi.app/) / [Pusher](https://pusher.com/), designed to work with Laravel Broadcasting and any Pusher SDK client.

## Features

- Full Pusher WebSocket protocol (connect, subscribe, unsubscribe, ping/pong)
- Channel types: public, private, presence, encrypted
- Client events (`client-*`) on private/presence channels
- Pusher-compatible HTTP API (trigger events, batch, channel queries)
- HMAC-SHA256 authentication (compatible with Laravel's BroadcastServiceProvider)
- Webhook delivery with batching
- Multi-app support
- Rate limiting (per-connection client events, per-app backend events)
- Max connection limit per app
- Max message size (default 100KB)
- Channel and event name validation
- CORS and WebSocket origin checking
- Activity timeout for idle connections
- TLS support
- Configurable log level
- Health check endpoint (`GET /health`)
- Stats endpoint (`GET /stats`) on a separate internal port with live Bubbletea CLI dashboard
- Graceful shutdown

## Quick Start

```bash
cp config.example.yaml config.yaml  # edit with your app credentials
go build -o gocket .
GOCKET_CONFIG_FILE=config.yaml ./gocket
```

Starts on port `6001`. The stats endpoint runs on a separate internal port (`6060`, bound to `127.0.0.1`).

## CLI

```bash
# Start the server
./gocket

# Live dashboard (connects to localhost:6060)
./gocket status

# Live dashboard (custom address)
./gocket status localhost:6060
```

## Configuration

Configure via a YAML config file (recommended) or environment variables. See `config.example.yaml` for all options.

```bash
cp config.example.yaml config.yaml
```

```yaml
host: 0.0.0.0
port: 6001
stats_port: 6060
log_level: info
max_message_size: 102400

apps:
  - id: myapp
    key: my-key
    secret: my-secret
    client_events: true
    max_connections: 1000
    max_client_events_per_sec: 10
    max_backend_events_per_sec: 100
```

```bash
GOCKET_CONFIG_FILE=config.yaml ./gocket
```

JSON config files are also supported (detected by file extension).

### Environment Variables

Environment variables override config file values.

| Variable | Default | Description |
|----------|---------|-------------|
| `GOCKET_CONFIG_FILE` | — | Path to YAML or JSON config file |
| `GOCKET_HOST` | `0.0.0.0` | Bind address |
| `GOCKET_PORT` | `6001` | Listen port |
| `GOCKET_STATS_PORT` | `6060` | Stats endpoint port (bound to `127.0.0.1` only) |
| `GOCKET_LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |
| `GOCKET_APPS` | — | JSON array of app configs |
| `GOCKET_TLS_CERT` | — | Path to TLS certificate |
| `GOCKET_TLS_KEY` | — | Path to TLS private key |
| `GOCKET_MAX_MESSAGE_SIZE` | `102400` | Max WebSocket message size in bytes (100KB) |
| `GOCKET_ALLOWED_ORIGINS` | — | Comma-separated allowed origins for CORS/WebSocket |

### Per-App Config Fields

| Field | Default | Description |
|-------|---------|-------------|
| `id` | — | App identifier |
| `key` | — | App key (used in WebSocket URL) |
| `secret` | — | App secret (HMAC signing) |
| `webhook_url` | — | Webhook delivery URL (must resolve to a public IP) |
| `client_events` | `false` | Allow `client-*` events |
| `max_connections` | `0` | Max concurrent connections (0 = unlimited) |
| `max_client_events_per_sec` | `0` | Rate limit for client events per connection (0 = unlimited) |
| `max_backend_events_per_sec` | `0` | Rate limit for HTTP API events per app (0 = unlimited) |

## Laravel Setup

### Install

```bash
composer require pusher/pusher-php-server
```

### .env

```env
BROADCAST_DRIVER=pusher

PUSHER_APP_ID=app-id
PUSHER_APP_KEY=app-key
PUSHER_APP_SECRET=app-secret
PUSHER_HOST=127.0.0.1
PUSHER_PORT=6001
PUSHER_SCHEME=http
PUSHER_APP_CLUSTER=mt1
```

### config/broadcasting.php

```php
'pusher' => [
    'driver' => 'pusher',
    'key' => env('PUSHER_APP_KEY'),
    'secret' => env('PUSHER_APP_SECRET'),
    'app_id' => env('PUSHER_APP_ID'),
    'options' => [
        'host' => env('PUSHER_HOST', '127.0.0.1'),
        'port' => env('PUSHER_PORT', 6001),
        'scheme' => env('PUSHER_SCHEME', 'http'),
        'encrypted' => false,
        'useTLS' => false,
        'cluster' => env('PUSHER_APP_CLUSTER'),
    ],
],
```

### Frontend (Laravel Echo)

```bash
npm install laravel-echo pusher-js
```

```js
import Echo from 'laravel-echo';
import Pusher from 'pusher-js';

window.Pusher = Pusher;
window.Echo = new Echo({
    broadcaster: 'pusher',
    key: 'app-key',
    wsHost: '127.0.0.1',
    wsPort: 6001,
    forceTLS: false,
    disableStats: true,
    enabledTransports: ['ws'],
    cluster: 'mt1',
});

// Listen on a public channel
Echo.channel('orders').listen('OrderCreated', (e) => {
    console.log(e);
});
```

## HTTP API

All endpoints require HMAC-SHA256 signature authentication via query parameters (`auth_key`, `auth_timestamp`, `auth_version`, `body_md5`, `auth_signature`).

| Method | Path | Description |
|--------|------|-------------|
| POST | `/apps/{appId}/events` | Trigger event (up to 100 channels) |
| POST | `/apps/{appId}/batch_events` | Trigger batch events (up to 10) |
| GET | `/apps/{appId}/channels` | List occupied channels |
| GET | `/apps/{appId}/channels/{channelName}` | Get channel info |
| GET | `/apps/{appId}/channels/{channelName}/users` | Get presence channel users |
| GET | `/health` | Health check (no auth) |
| GET | `/stats` | Server stats — served on internal port (`127.0.0.1:6060`), not exposed publicly |

## WebSocket Protocol

Connect to `ws://host:6001/app/{appKey}`.

On connect, the server sends:
```json
{"event":"pusher:connection_established","data":"{\"socket_id\":\"123.456\",\"activity_timeout\":120}"}
```

Subscribe to a channel:
```json
{"event":"pusher:subscribe","data":"{\"channel\":\"my-channel\"}"}
```

Ping/pong keepalive:
```json
{"event":"pusher:ping","data":"{}"}
```

## Limits and Validation

| Limit | Default |
|-------|---------|
| Max message size | 100 KB |
| Max channel name length | 200 chars |
| Max event name length | 200 chars |
| Channel name characters | `A-Z a-z 0-9 _ - = @ , . ;` |
| Max channels per connection | 100 |
| Max channels per trigger | 100 |
| Max events per batch | 10 |

## Benchmark

Load tested on a DigitalOcean droplet (1 CPU, 2 GB RAM) with [k6](https://k6.io/):

| Metric | Value |
|--------|-------|
| Subscribers | 100 concurrent WebSocket connections |
| Publish rate | 50 events/sec |
| Total messages delivered | 900,000 |
| Throughput | 3,750 msgs/sec |
| Median delivery latency | 8 ms |
| p95 delivery latency | 33 ms |
| Median HTTP publish | 3.19 ms |
| Avg WebSocket connect time | 4.39 ms |
| Failed requests | 0% |

## Project Structure

```
main.go                          # Entrypoint, subcommands, routing, graceful shutdown
config.go                        # Config loading (YAML/JSON file, env vars)
internal/
  app/                           # App struct + registry (lookup by key/ID)
  proto/                         # Shared Event type, Subscriber interface, validation
  ws/                            # WebSocket connection wrapper + per-conn rate limiter
  server/                        # WebSocket handler (Pusher protocol lifecycle)
  channel/                       # Channel types, subscribe/unsubscribe, broadcast
  auth/                          # HMAC-SHA256 signing + token validation
  api/                           # HTTP API (events, channels, stats, CORS, auth middleware)
  webhook/                       # Async webhook dispatcher
  state/                         # Per-app connection tracking + max connection enforcement
  ratelimit/                     # Token bucket rate limiter
  tui/                           # Bubbletea CLI dashboard
```

## License

MIT
