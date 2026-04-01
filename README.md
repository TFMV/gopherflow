# GopherFlow

A lightweight distributed task scheduler and workflow orchestrator in Go.

## Features

- **In-memory priority queue** with heap-based scheduling
- **Pluggable executors**: Shell, HTTP, Echo
- **Worker pool** with configurable concurrency
- **Exponential backoff retry** with Dead Letter Queue
- **Real-time TUI** dashboard (BubbleTea)
- **Prometheus-compatible metrics**
- **Minimal dependencies** - Go standard library + ULID + BubbleTea

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   API       │────▶│   Queue      │────▶│   Workers   │
│  (HTTP)     │     │ (Priority)  │     │   (Pool)    │
└─────────────┘     └──────────────┘     └─────────────┘
                           │                    │
                           ▼                    ▼
                    ┌──────────────┐     ┌─────────────┐
                    │  JobStore    │     │ Executors   │
                    │  (In-memory) │     │ (Shell/HTTP)│
                    └──────────────┘     └─────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │   Runtime    │
                    │   State      │
                    └──────────────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
        ┌──────────┐             ┌──────────┐
        │ /metrics │             │    TUI   │
        └──────────┘             └──────────┘
```

## Installation

```bash
git clone https://github.com/TFMV/gopherflow.git
cd gopherflow
go build ./...
```

## Usage

### Server Mode (API + Workers)

```bash
go run . server
# or with flags
go run . server --port 9000 --workers 5
```

**API Endpoints:**

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/jobs` | Submit a new job |
| GET | `/jobs` | List jobs (supports `?limit=N&offset=N`) |
| GET | `/jobs/{id}` | Get job by ID |
| DELETE | `/jobs/{id}` | Cancel a job |
| GET | `/health` | Health check |
| GET | `/metrics` | Prometheus metrics |
| GET | `/state` | Full runtime state |

**Submit a job:**

```bash
# Echo job
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"echo","payload":{"message":"hello"},"priority":1,"max_retries":3}'

# Shell command
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"shell","payload":{"command":"echo hi"},"priority":1,"max_retries":3}'

# HTTP request
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"http","payload":{"method":"GET","url":"https://httpbin.org/get"},"priority":1,"max_retries":3}'
```

**Job payload schema:**

```json
{
  "type": "echo|shell|http",
  "payload": { ... },
  "priority": 1,
  "max_retries": 3
}
```

### Submit Command (CLI)

```bash
go run . submit --type echo --payload '{"message":"hello"}'
go run . submit --type shell --payload '{"command":"ls -la"}'
go run . submit --type http --payload '{"url":"https://example.com"}'
```

### TUI Mode (Dashboard)

```bash
go run . ui
```

Controls:
- `up/down` - Select job
- `q` / `ctrl+c` - Quit

### Worker Mode (Standalone)

```bash
go run . worker
```

## Configuration

**Command-line flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | 8080 | API server port |
| `-workers` | 3 | Number of workers |
| `-server` | http://localhost:8080 | Server URL for UI/submit |
| `-type` | echo | Job type for submit |
| `-payload` | {} | Job payload JSON |
| `-priority` | 1 | Job priority |
| `-retries` | 3 | Max retries |
| `-version` | false | Show version |

**System parameters (code):**

- **MaxRetries**: 3 (configurable per job)
- **RetryBackoff**: 2s base, exponential (2^n seconds)
- **Queue**: Thread-safe heap-based priority queue

## Job Status Lifecycle

```
queued → running → succeeded
                → failed → retrying → queued (up to MaxRetries)
                → failed → dead (after MaxRetries exhausted)
```

## Project Structure

```
gopherflow/
├── core/
│   ├── domain.go      # Job, Execution, Workflow models
│   ├── queue.go       # Priority queue implementation
│   ├── scheduler.go   # Scheduler with retry logic
│   ├── worker.go      # Worker and WorkerPool
│   ├── executors.go   # Shell, HTTP, Echo executors
│   ├── state.go       # RuntimeState and Metrics
│   └── interfaces.go  # Core interfaces
├── api/
│   └── handler.go     # HTTP handlers
├── tui/
│   └── dashboard.go   # BubbleTea TUI
└── main.go            # CLI entry point
```

## Metrics

Prometheus-format output at `/metrics`:

```
gopherflow_jobs_total{status="queued"} 0
gopherflow_jobs_total{status="running"} 2
gopherflow_jobs_total{status="succeeded"} 10
gopherflow_jobs_in_flight 2
gopherflow_worker_count 3
gopherflow_job_duration_seconds_avg 0.123
```

## License

MIT