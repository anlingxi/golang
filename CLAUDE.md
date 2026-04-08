# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working in this repository.

## Current Repo Reality

This workspace is **not** a clean single-root monorepo anymore.

- The repo root contains a partial snapshot: `cmd/`, `configs/`, `deployments/`, `docs/`, `frontend/`, `homepage/`, `go.mod`, `README.md`.
- The **complete backend** and the **buildable homepage app** live in the nested directory: `paismart-go-main/`.
- The root-level Go backend is incomplete: it has `cmd/server/main.go`, but it does **not** have root-level `internal/` or `pkg/`, so backend build commands fail from the repo root.
- Several files are duplicated between the repo root and `paismart-go-main/` (for example `AGENTS.md`, `CLAUDE.md`, `docs/`, `deployments/`, `frontend/`).

When working on backend code, treat `paismart-go-main/` as the source-of-truth application root unless the task explicitly targets the outer snapshot.

## Working Directories

- Backend: `cd paismart-go-main`
- Frontend: use repo-root `frontend/` by default; the same app is also duplicated under `paismart-go-main/frontend/`
- Homepage: use `cd paismart-go-main/homepage`; repo-root `homepage/` only contains assets, not a complete app
- Infra: `deployments/docker-compose.yaml` and `paismart-go-main/deployments/docker-compose.yaml` are duplicates

## Actual Code Structure

### Outer repo root

The outer root currently contains:

- Partial Go snapshot: `cmd/`, `configs/`, `deployments/`, `docs/`
- Full frontend workspace: `frontend/`
- Asset-only homepage snapshot: `homepage/`
- A nested full application copy: `paismart-go-main/`

Important:

- Root `go.mod` is a minimal snapshot and does not reflect the real backend dependency graph.
- Root `cmd/server/main.go` imports `pai-smart-go/internal/...` and `pai-smart-go/pkg/...`, but those packages do not exist at the outer root.

### Nested backend root: `paismart-go-main/`

This is the actual backend/application tree:

```text
paismart-go-main/
├── cmd/server/main.go
├── configs/config.yaml
├── deployments/docker-compose.yaml
├── docs/ddl.sql
├── frontend/
├── homepage/
├── initfile/
├── internal/
│   ├── ai/
│   │   ├── helper/
│   │   └── history/
│   ├── app/bootstrap/
│   ├── config/
│   ├── eino/
│   │   ├── callbacks/
│   │   ├── chat/
│   │   ├── document/
│   │   ├── factory/
│   │   ├── tools/
│   │   └── types/
│   ├── handler/
│   ├── infra/
│   │   ├── cache/
│   │   └── mq/rabbitmq/
│   ├── middleware/
│   ├── model/
│   ├── pipeline/
│   ├── repository/
│   ├── router/
│   ├── seed/
│   └── service/
└── pkg/
    ├── code/
    ├── database/
    ├── embedding/
    ├── es/
    ├── hash/
    ├── kafka/
    ├── log/
    ├── storage/
    ├── tasks/
    ├── tika/
    └── token/
```

## Architecture Notes That Match The Current Code

The older docs describe a simpler Gin + Kafka + RAG backend. That is only partially true now.

### Still true

- Gin is still the HTTP framework.
- MySQL, Redis, MinIO, Tika, Kafka, and Elasticsearch are still part of the stack.
- The document ingestion path still uses Kafka plus `internal/pipeline/`.
- The app still uses layered `handler -> service -> repository` organization.

### Newly present in the checked-in code

- `internal/eino/` introduces a newer AI integration layer with:
  - provider factories
  - chat model adapters
  - document pipeline builders
  - tool builders
  - callback plumbing
- `internal/ai/helper/` and `internal/ai/history/` add helper/session/history management.
- `internal/infra/mq/rabbitmq/` adds RabbitMQ producer/consumer code for async history persistence.
- `internal/handler/agent_handler.go` adds an agent chat path for `/api/v1/agent/chat`.
- `internal/router/router.go` centralizes route registration.
- `internal/seed/seed.go` seeds files from `initfile/`.

### Practical implication

Do not document this repo as “Kafka-only async processing” anymore.

Current code shows two async/message paths:

- Kafka for document processing / ingestion
- RabbitMQ for AI chat history persistence

Do not document this repo as “plain RAG chat only” either.

Current code also includes:

- Eino-based chat model abstraction
- tool-enabled agent chat
- helper/history management modules

## Frontend Reality

The repo-root `frontend/` is a complete Vue 3 + TypeScript + Vite app.

Observed structure includes:

- `frontend/src/views/chat`
- `frontend/src/views/chat-history`
- `frontend/src/views/knowledge-base`
- `frontend/src/views/org-tag`
- `frontend/src/views/personal-center`
- `frontend/src/views/user`
- `frontend/src/router/elegant/routes.ts`
- `frontend/packages/*` workspace packages:
  - `alova`
  - `axios`
  - `color`
  - `hooks`
  - `materials`
  - `ofetch`
  - `scripts`
  - `uno-preset`
  - `utils`

Frontend commands in the old docs are still broadly correct, but they should be run from the repo-root `frontend/`.

## Homepage Reality

The repo-root `homepage/` is not a complete standalone homepage app. It contains assets only.

The actual homepage app is in:

- `paismart-go-main/homepage/index.html`
- `paismart-go-main/homepage/index.js`
- `paismart-go-main/homepage/package.json`
- `paismart-go-main/homepage/vite.config.js`

If a task involves homepage development, work in `paismart-go-main/homepage/`.

## Commands That Match The Current Repo

### Backend

```bash
cd paismart-go-main
go mod tidy
go build ./cmd/server
go run cmd/server/main.go
```

Do **not** run backend build or run commands from the outer repo root.

### Frontend

```bash
cd frontend
pnpm install
pnpm run dev
pnpm run dev:prod
pnpm run build
pnpm run build:test
pnpm run typecheck
pnpm run lint
```

### Homepage

```bash
cd paismart-go-main/homepage
pnpm install
pnpm run dev
pnpm run build
```

### Infrastructure

```bash
docker compose -f deployments/docker-compose.yaml up -d
docker compose -f deployments/docker-compose.yaml down
```

## Current Breakage / Incomplete Refactor State

Do not assume the nested backend currently builds cleanly.

Observed problems in checked-in code:

- Merge conflict markers exist in:
  - `paismart-go-main/configs/config.yaml`
  - `paismart-go-main/internal/service/document_service.go`
  - `paismart-go-main/internal/service/search_service.go`
  - `paismart-go-main/internal/service/upload_service.go`
- `paismart-go-main/cmd/server/main.go` references newer Eino / agent wiring that does not fully match other checked-in files.
- The nested config struct and route registration code appear mid-refactor, so additional compile errors may exist even after removing conflict markers.

Outer-root backend build is already known-bad because the outer root is missing `internal/` and `pkg/`.

## Configuration Reality

Outer-root `configs/config.yaml` is an older, simpler snapshot.

Nested `paismart-go-main/configs/config.yaml` is the backend config under active change and includes newer sections for:

- Kafka chat-history topic
- Eino-related settings
- RabbitMQ
- document splitter settings

However, that nested config file currently contains unresolved conflict markers.

## Testing Reality

- There are currently no `*_test.go` files in the repository.
- Do not claim the project has automated backend test coverage.

## Guidance For Claude Code

- First determine whether the task targets the outer snapshot or the nested source-of-truth app.
- For backend work, default to `paismart-go-main/`.
- For homepage work, default to `paismart-go-main/homepage/`.
- For frontend work, default to repo-root `frontend/` unless the task explicitly targets the duplicated nested copy.
- Before saying the backend is runnable, verify from `paismart-go-main/`.
- Before editing config or service files in the nested backend, check for conflict markers with:

```bash
rg -n "^(<<<<<<<|=======|>>>>>>>)" paismart-go-main
```

- When documenting architecture, mention both:
  - Kafka document processing
  - RabbitMQ + Eino helper/history/agent modules

The older `AGENTS.md` and previous `CLAUDE.md` describe an earlier, cleaner layout and are no longer accurate enough to use as-is.

## Prohibited Git Operations
- DO NOT run git commit
- DO NOT run git push  
- DO NOT run git add
- Read-only git commands are allowed: git status / git diff / git log

## Scope of Work

- Only modify code as explicitly instructed — do not add unrequested features
- Do NOT run the backend server or any long-running process
- Do NOT run tests or test commands (go test, pnpm test, etc.)
- Do NOT run build commands unless explicitly asked
- Allowed read operations: read files, search code, understand structure
- Allowed write operations: edit files per instruction only

## Before Making Any Change

- State which file(s) you plan to modify and why
- Wait for confirmation before editing