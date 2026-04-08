# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working inside `paismart-go-main/`.

## Scope

This directory is the **actual backend application root** inside the outer repository.

If you are working here, use this directory as the base for:

- backend code
- backend config
- backend infrastructure files
- the full homepage app
- the duplicated frontend app

## Actual Structure

```text
.
├── cmd/server/main.go
├── configs/config.yaml
├── deployments/docker-compose.yaml
├── docs/ddl.sql
├── frontend/
├── homepage/
├── initfile/
├── internal/
│   ├── ai/
│   ├── app/bootstrap/
│   ├── config/
│   ├── eino/
│   ├── handler/
│   ├── infra/
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

## Architecture Reality

Older docs that describe a simple Gin + Kafka + RAG backend are incomplete.

Current checked-in code includes:

- classic `handler -> service -> repository` layering
- Kafka-based document ingestion
- `internal/eino/` for newer AI/chat/document/tool integration
- `internal/ai/helper/` and `internal/ai/history/` for helper/session/history management
- RabbitMQ code in `internal/infra/mq/rabbitmq/` for async history persistence
- centralized route registration in `internal/router/router.go`
- `internal/handler/agent_handler.go` for `/api/v1/agent/chat`
- seed import logic in `internal/seed/seed.go`

When describing the system, mention both:

- Kafka document processing
- RabbitMQ + Eino helper/history/agent flow

## Commands

### Backend

```bash
go mod tidy
go build ./cmd/server
go run cmd/server/main.go
```

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
cd homepage
pnpm install
pnpm run dev
pnpm run build
```

### Infrastructure

```bash
docker compose -f deployments/docker-compose.yaml up -d
docker compose -f deployments/docker-compose.yaml down
```

## Current Breakage

Do not assume this backend currently builds.

Observed issues:

- unresolved conflict markers in:
  - `configs/config.yaml`
  - `internal/service/document_service.go`
  - `internal/service/search_service.go`
  - `internal/service/upload_service.go`
- `cmd/server/main.go` references newer wiring that does not fully match the checked-in config and route definitions
- additional compile errors may exist after conflict cleanup because the Eino / agent refactor is not fully aligned yet

Before saying the backend is runnable, verify it again locally.

## Configuration Notes

This subtree's `configs/config.yaml` is the relevant backend config file.

It includes or attempts to include settings for:

- Kafka
- Eino
- RabbitMQ
- document splitting

However, the file currently has merge conflict markers and should be treated as mid-refactor.

## Testing Reality

- There are no `*_test.go` files in this subtree.
- Do not claim there is automated backend test coverage.

## Guidance For Claude Code

- Start backend work here, not from the outer repo root.
- Check for conflict markers before editing service or config files:

```bash
rg -n "^(<<<<<<<|=======|>>>>>>>)" .
```

- If a task involves homepage work, use `homepage/`.
- If a task involves duplicated frontend code, confirm whether the outer repo `frontend/` or this local `frontend/` is the intended target.
- If you need repo-wide layout context, also read the outer `../CLAUDE.md`.


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