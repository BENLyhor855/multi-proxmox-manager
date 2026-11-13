# Multi-Proxmox Manager

A centralized management control plane for 2–5 independent Proxmox VE installations, built with Angular, Go, and MySQL. The Go module path uses the reserved `example.com` placeholder; change it to your private Git repository path before publishing.

> **Current delivery: Phase 1 foundation.** Local Argon2id authentication, role checks, encrypted API-token storage, registration/testing of Proxmox endpoints, node discovery, and audit events are included. VM/LXC lifecycle and destructive administration operations are deliberately deferred to later phases.

## Technology choices

- Angular 21 LTS frontend
- Go 1.27.4 API service
- MySQL 8.0+ database (tested on 8.4.13)
- Nginx TLS termination and same-origin reverse proxy
- Proxmox VE REST API using scoped API tokens

## Repository map

```text
cmd/
  api/                 Go API executable
  create-admin/        one-time local administrator bootstrap
  create-user/         CLI bootstrap for operator/viewer accounts
internal/
  config/              environment validation
  database/            MySQL connection pool
  httpapi/             routes, authentication, RBAC, server handlers
  proxmox/             TLS-verified, CIDR-restricted Proxmox client
  security/            AES-GCM secrets and signed sessions
migrations/
  001_init.sql         initial MySQL schema
deploy/
  nginx/               HTTPS virtual host example
  systemd/             hardened systemd unit
docs/
  architecture.md      component map and later-phase plan
  phase-1-runbook.md   deployment steps and acceptance checklist
web/                   Angular application
.env.example           environment template (never store real secrets here)
```

## Quick start

Use the full deployment instructions in [`docs/phase-1-runbook.md`](docs/phase-1-runbook.md). High-level sequence:

1. Prepare an existing Ubuntu VM. Keep it separate from the Proxmox hypervisor host.
2. Install Go 1.27.4, Node.js 24.22.1 (or a supported Angular 21 Node version), MySQL 8.0+ and Nginx.
3. Create the database and application user; apply both SQL files in `migrations/` in numeric order.
4. Configure `/etc/multi-proxmox-manager/mmp.env` with independent signing/encryption keys and narrow `PVE_ALLOWED_CIDRS`.
5. Build `cmd/api`, create the first local admin with `cmd/create-admin`, and start the systemd service.
6. Build the Angular app and publish it through Nginx over HTTPS.
7. Create local operator/viewer accounts with `cmd/create-user` as required.
8. Create a least-privilege `PVEAuditor` API token on each PVE endpoint and register them in the web UI.

## Local development

Backend:

```bash
cp .env.example .env
# Edit .env and configure a local MySQL DSN, keys, CIDRs, and APP_ENV=development.
set -a && source .env && set +a
go mod download
go run ./cmd/api
```

Frontend in a second terminal:

```bash
cd web
npm ci
npm start
```

The Angular development server proxies `/api` to `http://127.0.0.1:8080` using `web/proxy.conf.json`.

For local HTTP development, set `COOKIE_SECURE=false`, `APP_ENV=development` and `APP_ORIGIN=`. Never use `COOKIE_SECURE=false` in production.

## API endpoints (Phase 1)

| Method | Path | Role | Purpose |
|---|---|---|---|
| `GET` | `/api/v1/healthz` | Public | Liveness/database health |
| `POST` | `/api/v1/auth/login` | Public | Local login |
| `GET` | `/api/v1/auth/me` | Any signed-in user | Current principal |
| `POST` | `/api/v1/auth/logout` | Any signed-in user | Revoke session and clear cookies |
| `GET` | `/api/v1/proxmox/servers` | Any signed-in user | List installations, sanitized |
| `POST` | `/api/v1/proxmox/servers` | `admin` | Register and test an installation |
| `POST` | `/api/v1/proxmox/servers/{id}/test` | `admin`, `operator` | Test PVE API connection |
| `DELETE` | `/api/v1/proxmox/servers/{id}` | `admin` | Remove from manager only |
| `GET` | `/api/v1/audit-logs?limit=100` | `admin` | Read recent audit events (max 200) |

All authenticated state-changing endpoints require the CSRF header `X-CSRF-Token`, matching the `mmp_csrf` cookie and the hash stored for that session. The Angular interceptor sends it automatically.

## Roles

- `admin`: register, test, and remove Proxmox endpoints.
- `operator`: test connections; later phases will add narrowly scoped operations.
- `viewer`: read-only view of registered endpoints.

The initial bootstrap command creates one `admin`. The `cmd/create-user` CLI can create `operator` and `viewer` accounts. In-app user management and self-service password change are planned for a later hardening step.

## Important security notes

- Do not use the Proxmox `root@pam` account as the application integration identity.
- Keep API token privilege separation enabled. The initial API token should be read-only (`PVEAuditor`).
- Local user passwords are hashed using Argon2id. Login failures trigger account lockout, with an additional Nginx per-IP login rate limit.
- PVE token secrets are stored encrypted. The `CREDENTIALS_ENCRYPTION_KEY` must be backed up securely and kept separate from database backups.
- TLS certificate verification is mandatory. Supply the issuing CA to `PVE_CA_FILE` for internal/self-signed certificate environments.
- Configure only the intended PVE management networks in `PVE_ALLOWED_CIDRS`; the connector checks resolved destination IPs during each TCP dial and only permits port 8006.
- Do not expose the Go API port 8080 or MySQL port 3306 publicly.
- Removing an installation only removes its record from this manager; it does not change or delete resources in Proxmox VE.

## Known Phase 1 limitations

- One local bootstrap administrator; user creation/role management UI is not included yet.
- No refresh-token flow; session cookie lifetime is 15 minutes and users sign in again after expiry. Sessions are revocable in MySQL and invalidated on logout.
- Proxmox inventory is currently limited to version and node list, not full VM/LXC/storage inventory.
- Successful authentication and state changes commit atomically with audit events; failed sign-ins are recorded without passwords. An immutable/off-host sink and automatic retention are not included.
- No HA deployment, scheduled polling, metrics history, or backup operations yet.

## Version notes

Version pins are intended for the 2026-11 deployment baseline. Re-check the official Go and Angular compatibility/support pages when upgrading the project toolchain. Keep `go.mod`, npm lockfiles, and OS package updates under change control.

## Completed Phase 1 delivery

See [implementation report](docs/implementation-report.md), [authentication flow](docs/authentication.md), [verification instructions](docs/testing.md), and the updated [Ubuntu Server 24.04 runbook](docs/phase-1-runbook.md). Browser admin controls include recent audit events. The development proxy preserves the browser Host for origin checks.

`make verify` runs Go tests/vet/build plus a reproducible Angular production build. Set `TEST_MYSQL_DSN` to an isolated disposable database to include integration tests; otherwise that suite is explicitly skipped. No real PVE installations were available during implementation: independent TLS API fixtures were tested, and live acceptance remains a deployment step.

The npm overrides for `glob` and `minimatch` select patched versions for development tools without upgrading Angular 21. Keep them under review when updating the CLI. No production secrets, node_modules or generated test databases are included in the source delivery.
