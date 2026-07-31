# Repository Guidelines

## Project Structure & Module Organization

Phoenix is an Ontology-first document intelligence monorepo. `backend/` contains the Go workflow service: entry points are under `cmd/`, while domain packages live in `internal/`. Configuration-driven models are in `backend/configs/doctypes/` and `backend/configs/ontology/`. `frontend/` is the Next.js administration console; routes are in `src/app/`, components in `src/components/`, and API clients in `src/lib/`. `phoenix-doc-assistant/` contains the WorkBuddy agent, skills, and Python client. Shared JSON contracts live in `contracts/`. Deployment files are in `deploy/`; requirements are in `docs/`.

## Build, Test, and Development Commands

- `make build`: compile all Go packages.
- `make test`: run the Go test suite.
- `make vet`: run Go static analysis.
- `make infra-up`: start PostgreSQL/pgvector, MinIO, and Redis.
- `make run-workflow`: run the backend on port 8081.
- `make fe-install && make fe-dev`: install frontend dependencies and run Next.js on port 8084.
- `make fe-build`: create the production static frontend export.
- `make smoke`: exercise upload, validation, storage, query, and Ontology materialization.

## Coding Style & Naming Conventions

Format Go with `gofmt`; package names are short lowercase nouns and tests use `*_test.go`. Follow existing Chinese domain terminology in comments and user-facing text. TypeScript uses two-space indentation, PascalCase components, camelCase functions, and kebab-case route/component filenames. Python uses four spaces, snake_case, and standard-library dependencies only. Keep document types and Ontology identifiers lowercase snake_case, such as `resolution_keys` and `warn_duplicate`.

## Testing Guidelines

Add table-driven Go tests beside changed packages and name them `TestBehaviorCondition`. Cover validation boundaries, normalization, identity, entity materialization, and API compatibility. Run `make test`, `make vet`, `make fe-build`, and `git diff --check` before submitting. Changes to JSON contracts must include or update an example under `contracts/examples/`.

## Commit & Pull Request Guidelines

History follows concise Conventional Commit-style subjects, for example `feat: implement ontology layer`, `fix: persist doc_type`, and `docs: update product guide`. Keep each commit focused. Pull requests should explain the customer outcome, affected Ontology objects/links/actions, migrations, API compatibility, and verification commands. Include screenshots for UI changes and sample request/response JSON for REST changes.

## Architecture, Security & Compatibility

WorkBuddy performs recognition; the backend archives, validates, stores, materializes Ontology objects, and retrieves data. Treat `/pub/v1` as a stable external contract—extend it instead of changing existing shapes. Never restore archived MCP components. Do not commit tokens, production passwords, `.env` files, or WorkBuddy `.config.json`. Preserve evidence and auditability when correcting extracted fields.
