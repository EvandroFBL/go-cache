# go-cache

An in-memory key-value cache with TTL (time-to-live), built from scratch in Go. **Zero external dependencies** — pure stdlib.

This project was built as a learning exercise for Go concurrency patterns: race conditions, mutexes, atomic operations, and goroutine synchronization.

## Features

- **Thread-safe** — concurrent reads/writes protected by `sync.RWMutex`
- **TTL expiration** — items auto-expire after a configurable duration
- **Background cleanup** — a goroutine periodically reaps expired keys
- **Atomic stats** — hit/miss/set/delete counters using `sync/atomic`
- **Cookie-based user isolation** — each user gets a unique session cookie; keys are namespaced per user so users cannot access each other's data
- **HTTP API** — RESTful endpoints using Go 1.22+ enhanced mux (no frameworks)
- **Graceful shutdown** — handles SIGINT/SIGTERM cleanly
- **Docker ready** — multi-stage Dockerfile + docker-compose with health checks

## Quick Start

### Docker (recommended)

```bash
docker-compose up -d
# API available at http://localhost:5456
```

### Go

```bash
cd ~/go-cache
go run .

# Or build and run
go build -o go-cache .
./go-cache
```

Server starts on `:8080` by default (or `:5456` via Docker).

## API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/cache` | List all non-expired keys **for current user** |
| `GET` | `/cache/{key}` | Get a value by key |
| `PUT` | `/cache/{key}` | Set a value (JSON body) |
| `DELETE` | `/cache/{key}` | Delete a key |
| `GET` | `/stats` | Cache statistics (hits, misses, hit rate) — global |

### User Isolation (Cookies)

Each user is identified by a `cache_session` cookie (32-char hex ID, HttpOnly, 90-day expiry). Keys are internally namespaced as `user:{id}:{key}`, so:

- User A sets `secret` → User B gets a 404 trying to read it
- User A lists keys → only sees their own
- User A deletes a key → does not affect User B's keys with the same name

The cookie is set automatically on the first request. No login required.

### Examples

```bash
# Set a key with 2-minute TTL (cookie is set automatically)
curl -c cookies.txt -X PUT localhost:5456/cache/greeting \
  -H 'Content-Type: application/json' \
  -d '{"value":"hello world","ttl":"2m"}'

# Get it (send the cookie back)
curl -b cookies.txt localhost:5456/cache/greeting

# List your keys
curl -b cookies.txt localhost:5456/cache

# Check global stats
curl localhost:5456/stats

# Delete
curl -b cookies.txt -X DELETE localhost:5456/cache/greeting
```

### PUT Body Format

```json
{
  "value": "any JSON value (string, number, object, array)",
  "ttl": "5m"
}
```

TTL uses Go duration format: `30s`, `5m`, `1h`, `2h30m`. Default is `5m`.

## Configuration

All via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `CLEANUP_INTERVAL` | `10s` | How often the reaper runs |
| `MAX_KEYS` | `0` (unlimited) | Max number of keys in cache |

```bash
PORT=3000 MAX_KEYS=1000 CLEANUP_INTERVAL=30s ./go-cache
```

## Testing

```bash
# Run all tests
go test -v ./...

# Run with the RACE DETECTOR (the whole point!)
go test -race -v ./...

# Run only the race condition stress tests
go test -race -run TestRace -v ./...

# Run only the cookie isolation tests
go test -race -run TestCookie -v ./...
```

## Project Structure

```
go-cache/
├── main.go              # Entry point, HTTP server, graceful shutdown
├── store.go             # Core cache — RWMutex + map (the concurrency heart)
├── item.go              # CacheItem struct with TTL logic
├── stats.go             # Atomic counters for cache metrics
├── cleanup.go           # Background TTL reaper goroutine
├── auth.go              # Cookie-based user ID + key namespacing
├── handler.go           # HTTP handlers (Go 1.22+ enhanced mux)
├── store_test.go        # Race condition stress tests
├── auth_test.go         # Cookie isolation tests
├── Dockerfile           # Multi-stage build (Go alpine → alpine runtime)
├── docker-compose.yml   # Docker Compose with health check
├── .dockerignore        # Build context filter
└── go.mod               # Module definition (zero external deps)
```

## Race Condition Hotspots

The interesting concurrency challenges in this codebase:

1. **`store.go`** — `Get()` uses `RLock` (shared read), `Set()`/`Delete()` use `Lock` (exclusive write). Without this, concurrent map access panics.

2. **`cleanup.go`** — The reaper goroutine acquires a write lock to delete expired keys while request handlers hold read locks. Too much cleanup = lock contention.

3. **`stats.go`** — `Hits.Add(1)` looks simple but `hits++` is NOT atomic — two goroutines can read the same value and both write `N+1`. The `sync/atomic` package prevents this.

4. **`store_test.go`** — The `TestRace*` tests intentionally stress these patterns. Run with `-race` to see Go's detector in action.

## Learning Exercises

Try these to deepen your understanding:

1. **Break it** — Remove `RLock()`/`RUnlock()` in `Get()`, run `go test -race`, watch the panic
2. **Swap to `sync.Map`** — Replace `map + RWMutex` with `sync.Map`, benchmark the difference
3. **Add sharding** — Split into 16 buckets with per-bucket locks, reduce contention under load
4. **Add benchmarks** — `func BenchmarkGet(b *testing.B)` to measure ops/sec
5. **Add LRU eviction** — When `MAX_KEYS` is reached, evict the least recently used item

## License

MIT
