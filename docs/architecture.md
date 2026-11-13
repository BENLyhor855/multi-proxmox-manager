# Architecture and Phase Plan

## Phase 1 boundary

Phase 1 provides the control-plane foundation:

- Local administrator login, short-lived signed sessions backed by revocable MySQL session hashes, HttpOnly session cookies, CSRF validation, and roles (`admin`, `operator`, `viewer`).
- Registration of 2–5 independent Proxmox VE endpoints.
- Proxmox API-token encryption at rest using AES-256-GCM.
- Outbound destination allowlisting and TLS certificate verification.
- Connection testing against the Proxmox version and node-list APIs.
- Audit events for login/logout and Proxmox registration, test, and removal.
- Angular UI for login and initial server inventory.

Phase 1 intentionally does **not** execute power, delete, snapshot, storage, backup, or cluster-changing operations. The long-term product scope includes those operations, but they will be implemented in later phases with operation-specific permission checks, confirmation flows, task polling, and audit records.

## Logical components

```text
Browser
  |
  | HTTPS (same origin; HttpOnly session + CSRF header)
  v
Nginx (TLS termination, static Angular assets, /api reverse proxy)
  |
  v
Go API (127.0.0.1:8080)
  |-- Local Argon2id authentication + RBAC
  |-- Proxmox inventory/connection service
  |-- AES-256-GCM credential encryption
  |-- Audit writer
  |
  +---- MySQL 8.0+ (users, proxmox_servers, audit_logs)
  |
  +---- HTTPS :8006 to each allowlisted Proxmox VE endpoint
          PVE-A / PVE-B / PVE-C / PVE-D / PVE-E
```

Each registered URL can represent a standalone PVE node or a PVE cluster endpoint. A PVE cluster is represented once through a cluster management endpoint; do not register every cluster node as a separate duplicate installation unless you specifically want node-level independent access.

## Security boundaries

- The browser never receives Proxmox API secrets.
- The token secret is encrypted using AES-256-GCM and a fresh random nonce. The encryption key is held outside MySQL in a root-readable environment file.
- TLS verification is on. For internal/self-signed PVE certificates, provide a trusted CA bundle with `PVE_CA_FILE`.
- `PVE_ALLOWED_CIDRS` is mandatory. Every DNS result and actual TCP destination is checked at dial time. The connector only allows HTTPS port 8006 and never follows redirects or proxy environment variables.
- Nginx is the only internet-facing service. The Go service binds to loopback and MySQL should bind to loopback or a protected private interface.
- Use a dedicated API identity per PVE endpoint with privilege separation enabled. Phase 1 uses the read-only `PVEAuditor` role. Do not start with `root@pam` or an unrestricted token.
- The first administrator is created once using the bootstrap command. The command refuses to create a second administrator when an admin already exists.

## Phase 2 — Read-only inventory

Poll cluster resources, nodes, QEMU VMs, LXC containers, storage, pools and task state. Normalize resources into tables, retain source installation IDs, and provide per-installation drill-down. Polling must be bounded, concurrent with a configurable limit, and use incremental timestamps where available.

## Phase 3 — Full operations

Add VM/LXC lifecycle operations, snapshots, console link-outs, storage controls, backup visibility, and user management. Each operation must have a distinct permission mapping, a durable job record, an idempotency key, a task UPID tracker, timeouts, and a before/after audit event. Destructive actions require explicit confirmation and should initially be restricted to the `admin` role.

## Phase 4 — Production operations

Add metrics history, alert rules, notifications, scheduled reports, credential rotation workflows, backup/recovery procedures, high availability for the management app, and optional SSO/LDAP.

## Phase 1 implementation additions

Migration 002 introduces a sessions table containing only SHA-256 bearer and CSRF hashes plus user/expiry. Authentication reloads role and active status on every request; JWT role claims are informational. Logout deletes its session row, making a captured cookie unusable after logout. The browser sends a session-bound CSRF token and production origin validation compares against APP_ORIGIN. The PVE client never uses local-user JWTs: it decrypts the corresponding installation API token server-side and sends it only over verified HTTPS port 8006.

State mutations and successful auth events commit with their audit rows in MySQL transactions. Account lockout updates are serialized with SELECT FOR UPDATE; successful login cannot race past a newly established lockout. Login hashing has bounded concurrency and per-source-IP request limits in addition to Nginx controls. Unknown/inactive/locked-account failures use a dummy Argon2 verification and best-effort audit logging.
