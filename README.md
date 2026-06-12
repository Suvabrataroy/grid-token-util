# Grid AI Token Utilization — Distributed AI based Coding Platform

  Grid Computing is an open-source distributed platform that intelligently routes AI coding tasks to idle developer
  machines across your organisation / dev teams . Instead of centralising expensive API keys on a single server, tasks are executed
  locally on worker nodes using pluggable AI agent adapters — supporting Claude, GitHub Copilot, Google Gemini,
  ChatGPT, and custom agents.

  Built with Go for the control plane and cross-platform worker daemon, and React 18 / TypeScript for the real-time
  dashboard, the platform offers enterprise-grade features including:

  - Smart task scheduling with Redis-backed queuing and PostgreSQL persistence
  - Multi-agent support — route each task to the right AI tool
  - Brownie Points leaderboard to gamify contributor participation
  - Security-first design — Argon2id API key hashing, HMAC output integrity, pre/post secret scanning, and immutable
  audit logs
  - Cross-platform workers — runs as a system service on Linux (systemd), macOS (launchd), and Windows
  - Production-ready deployment — Docker Compose, Kubernetes, Prometheus metrics, and Grafana dashboards included

  Ideal for engineering teams looking to harness spare compute capacity while keeping AI workflows private, auditable,
  and cost-efficient.

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                        Control Plane                             │
│  REST API  ·  Task Scheduler  ·  Brownie Points  ·  Dashboard    │
│  PostgreSQL 16  ·  Redis 7.2  ·  React/TypeScript UI             │
└───────────────────────────┬──────────────────────────────────────┘
                            │  HTTPS + API Key
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                 ▼
   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐
   │ Grid Worker │   │ Grid Worker │   │ Grid Worker │
   │  (Linux)    │   │  (macOS)    │   │  (Windows)  │
   └─────────────┘   └─────────────┘   └─────────────┘
   systemd daemon     launchd daemon    Windows Service
   Claude adapter     Copilot adapter   Gemini adapter
```

### Components

| Component | Location | Description |
|---|---|---|
| **control-plane** | `control-plane/` | Go server — task queue, scheduler, REST API, React dashboard |
| **grid-worker** | `grid-worker/` | Go daemon — executes tasks via AI agent adapters |

---

## Quick Start (Windows)

**Start the server:**
```bat
run-server.bat
```

**Start a worker (separate terminal):**
```bat
run-worker.bat
```

Both scripts auto-check prerequisites and attempt to install missing dependencies via `winget`.

---

## Control Plane

### Prerequisites

- Go 1.22+
- PostgreSQL 16 (service running, database created)
- Redis 7.2 (service running)

### Configuration

```yaml
# control-plane/config.example.yaml
server:
  port: 8080
  hmac_secret: "change-me-in-production"

database:
  dsn: "postgres://user:pass@localhost:5432/gridcomputing?sslmode=disable"

redis:
  addr: "localhost:6379"

scheduler:
  tick_sec: 5

heartbeat:
  ttl_sec: 60

rate_limit:
  per_minute: 120
```

Copy `config.example.yaml` to `config.yaml` and edit before starting.

### Database Setup

```bash
# Apply migrations in order
psql -U postgres -d gridcomputing -f migrations/001_create_orgs.sql
psql -U postgres -d gridcomputing -f migrations/002_create_workers.sql
psql -U postgres -d gridcomputing -f migrations/003_create_tasks.sql
psql -U postgres -d gridcomputing -f migrations/004_create_api_keys.sql
psql -U postgres -d gridcomputing -f migrations/005_create_audit_log.sql
psql -U postgres -d gridcomputing -f migrations/006_create_outputs.sql
psql -U postgres -d gridcomputing -f migrations/007_create_brownie.sql
```

Or via the migration utility:
```bash
cd control-plane
go run ./cmd/migrate
```

### Build & Run

```bash
cd control-plane
go build -o bin/control-plane.exe ./cmd/server
bin/control-plane.exe
```

### API Endpoints

All endpoints under `/api/v1` require an `Authorization: Bearer <api-key>` header.

| Method | Path | Scope | Description |
|--------|------|-------|-------------|
| GET | `/healthz` | — | Liveness probe |
| GET | `/readyz` | — | Readiness probe |
| GET | `/api/v1/tasks` | — | List tasks |
| POST | `/api/v1/tasks` | `tasks:write` | Submit a task |
| GET | `/api/v1/tasks/{id}` | — | Get task |
| PATCH | `/api/v1/tasks/{id}/status` | `tasks:write` | Update task state |
| GET | `/api/v1/workers` | — | List workers |
| GET | `/api/v1/workers/{id}` | — | Get worker |
| POST | `/api/v1/workers/register` | `workers:register` | Register worker |
| POST | `/api/v1/workers/{id}/heartbeat` | `workers:heartbeat` | Worker heartbeat (returns assigned task) |
| POST | `/api/v1/outputs` | `workers:heartbeat` | Submit task output |
| POST | `/api/v1/outputs/{id}/review` | `tasks:write` | Approve/reject output |
| POST | `/api/v1/api-keys` | `keys:write` | Create API key |
| POST | `/api/v1/api-keys/{id}/rotate` | `keys:write` | Rotate API key |
| DELETE | `/api/v1/api-keys/{id}` | `keys:write` | Revoke API key |
| GET | `/api/v1/audit` | — | Query audit log |
| GET | `/api/v1/brownie/leaderboard` | — | Brownie points leaderboard |
| GET | `/api/v1/dashboard/stream` | — | SSE dashboard stream |
| GET | `/api/v1/dashboard/snapshot` | — | Dashboard snapshot |
| GET | `/api/v1/orgs` | `admin` | List orgs |
| POST | `/api/v1/orgs` | `admin` | Create org |

### Dashboard UI

```bash
cd control-plane/dashboard-ui
npm install
npm run dev        # development server (proxies API)
npm run build      # production build (served by Go via embed)
```

---

## Grid Worker

### Prerequisites

- Go 1.22+
- Git 2.30+
- At least one AI agent binary on `PATH` (see [Adapters](#adapters))
- `grid-worker/config.yaml` (copy from `config.example.yaml`)

### Configuration

```yaml
# grid-worker/config.example.yaml
server:
  url: "http://localhost:8080"
  api_key: "gk-your-worker-api-key"

worker:
  name: "my-machine"           # human-readable label
  agents: ["claude", "gemini"] # agents this worker supports

workspace:
  base_dir: ""           # defaults to OS temp dir
  max_disk_mb: 2048

policy:
  auto_approve: false    # require human approval for tasks
  allowed_repos: []      # empty = allow all

security:
  scan_pre_execute: true
  scan_post_execute: true
  ruleset_path: ""       # empty = use embedded default ruleset
```

### Installation as a System Service

**Windows (run as Administrator):**
```bat
grid-worker.exe install
net start GridWorker
```

**Linux (systemd):**
```bash
sudo grid-worker install
sudo systemctl enable --now grid-worker
```

**macOS (launchd):**
```bash
sudo grid-worker install
sudo launchctl load /Library/LaunchDaemons/com.grid-worker.plist
```

### CLI Commands

```
grid-worker run          Start the worker (foreground)
grid-worker install      Install and start as a system service
grid-worker uninstall    Stop and remove the system service
grid-worker status       Show daemon state and current task
grid-worker pause        Pause task acceptance
grid-worker resume       Resume task acceptance
grid-worker approve      Approve a pending task (policy mode)
grid-worker revoke       Revoke a previously approved task
grid-worker set-key      Store API key in system keychain
grid-worker preflight    Run prerequisite checks only
grid-worker logs         Tail worker logs
```

### Adapters

The worker selects the adapter matching the `ai_agent` field on each assigned task.

| Agent | Binary | Install |
|---|---|---|
| **Claude** | `claude` | [claude.ai/code](https://claude.ai/code) |
| **GitHub Copilot** | `gh` (with copilot extension) | `gh extension install github/gh-copilot` |
| **Google Gemini** | `gemini` | [ai.google.dev/gemini-api/docs/gemini-cli](https://ai.google.dev/gemini-api/docs/gemini-cli) |
| **ChatGPT** | `openai` | `pip install openai-cli` |
| **Custom** | configurable | Set `adapter.binary` in config |

---

## Task Lifecycle

```
submitted → queued → assigned → running → completed
                                        ↘ failed
```

1. Client POSTs task to `/api/v1/tasks` (state: `queued`)
2. Scheduler picks idle worker, sets state: `assigned`
3. Worker learns of assignment via heartbeat response
4. Worker pulls repo, executes AI agent, scans output
5. Worker POSTs result to `/api/v1/outputs` (state: `running` → `completed`)
6. Reviewer approves/rejects output; Brownie Points awarded/deducted

---

## Security

- **API Keys**: Argon2id-hashed, prefix-indexed, per-key scopes
- **Worker identity**: SHA-256 hash of hostname (privacy-preserving)
- **Output integrity**: HMAC-SHA256 over submitted artifacts
- **Secret scanning**: Pre- and post-execution scans against configurable ruleset
- **Rate limiting**: Redis-backed sliding window, configurable per-minute limit
- **Audit log**: Immutable append-only log of all state-changing API calls
- **Multi-replica safety**: `pg_try_advisory_xact_lock` prevents duplicate scheduling

---

## Deployment

### Docker Compose (local dev)

```bash
cd control-plane/deployments
docker compose up -d
```

Starts PostgreSQL, Redis, the control-plane server, and an Nginx reverse proxy.

### Kubernetes

```bash
kubectl apply -f control-plane/deployments/k8s/
```

Includes HPA, PDB, Ingress, ConfigMap, and a CronJob for audit log purging.

### Monitoring

Prometheus scrapes `/metrics`; a pre-built Grafana dashboard JSON is at `deployments/grafana/dashboard.json`.

---

## Development

### Running Tests

```bash
# Control plane unit + integration tests
cd control-plane
go test ./...

# Load tests (requires k6)
k6 run tests/load/k6-scenario.js

# Grid worker tests
cd grid-worker
go test ./...
```

### CI

GitHub Actions workflows in `.github/workflows/`:
- `server-ci.yml` — lint, test, build control-plane on push/PR
- `worker-ci.yml` — lint, test, cross-compile grid-worker (linux/amd64, darwin/arm64, windows/amd64)
- `load-test.yml` — k6 load test on merge to main

### Linting

```bash
golangci-lint run   # in either module directory
```

---

## Project Structure

```
grid-computing/
├── control-plane/              Go server + React dashboard
│   ├── cmd/server/             Entry point
│   ├── cmd/migrate/            DB migration utility
│   ├── internal/
│   │   ├── api/                HTTP handlers, middleware, router
│   │   ├── brownie/            Brownie Points engine + ledger
│   │   ├── dashboard/          SSE hub, projector, snapshotter
│   │   ├── domain/             Shared types
│   │   ├── queue/              Redis task queue
│   │   ├── scheduler/          Task–worker matcher + reaper
│   │   ├── security/           API key hashing, HMAC, secret scan
│   │   └── store/              Postgres + Redis stores
│   ├── dashboard-ui/           React 18 / TypeScript / Vite / Tailwind
│   ├── migrations/             SQL migration files (001–007)
│   ├── deployments/            Docker Compose, k8s, Prometheus, Grafana
│   └── tests/                  Integration + load tests
│
├── grid-worker/                Cross-platform worker daemon
│   ├── cmd/grid-worker/        Entry point
│   ├── internal/
│   │   ├── adapters/           AI agent adapters (Claude, Copilot, Gemini, ChatGPT, Custom)
│   │   ├── cli/                Cobra CLI commands
│   │   ├── config/             Config loader + schema
│   │   ├── control/            IPC socket (Unix pipe / Windows named pipe)
│   │   ├── controlplane/       HTTP client for control-plane API
│   │   ├── daemon/             Main daemon loop, lifecycle, signal handling
│   │   ├── executor/           8-step task execution pipeline
│   │   ├── policy/             Approval policy engine
│   │   ├── preflight/          PF-01…PF-10 startup checks
│   │   ├── reporter/           Result packing + HMAC signing
│   │   ├── scanner/            Secret scan (pre/post execute)
│   │   └── workspace/          Git clone, disk quota management
│   └── pkg/platform/           OS-specific battery, paths, process priority
│
├── run-server.bat              Windows: prereq-check + build + start server
├── run-worker.bat              Windows: prereq-check + build + start worker
└── .github/workflows/          CI/CD pipelines
```
