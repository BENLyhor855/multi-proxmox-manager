# Phase 1 implementation report

Date: 13 November 2026 (Asia/Bangkok).

## Delivery status

The supplied Phase 1 source has been reviewed and modified, with Go binaries and the Angular production bundle successfully built. The documented Phase 1 scope—local authentication, RBAC, encrypted endpoint credentials, independent endpoint registration/testing and audit logging—is implemented and exercised against MySQL and independent TLS API fixtures.

**Real Proxmox VE acceptance remains outstanding.** No actual PVE endpoints, credentials or private network access were supplied. Two independent simulated PVE APIs were tested; this does not establish connectivity or compatibility with the user's real hypervisors. Ubuntu Server 24.04 installation instructions are provided, but the delivery environment ran Debian 13; a clean Ubuntu installation was not performed.

The original ZIP is the input baseline. Implementation and testing did not deploy the application or change any production database. GitHub publication is recorded in the repository commit history. No existing application capability was removed. VM/LXC operations, storage controls, scheduled polling and destructive PVE actions remain intentionally absent as specified by the architecture's Phase 1 boundary.

## Completed features and changes

| Area | Implemented behavior |
|---|---|
| Build correctness | Fixed the missing context import in authentication middleware; added Go/npm lockfiles; added the missing Angular development-server target; added Makefile targets that build extracted source without VCS metadata. |
| Local passwords | Retained fixed-parameter Argon2id with random salts and constant-time comparison; preserved 14–1024 byte creation policy; added comparable dummy hashing for unknown/inactive/locked accounts. |
| Sessions | Added migration 002 with hashed bearer/CSRF tokens, user and expiry; random JWT IDs make sessions unique; logout durably revokes the session; expiry, issuer, header, signature, signing-key strength and lifetime are validated. |
| Brute-force controls | Eight wrong passwords lock accounts for 15 minutes; row locking serializes verification/count updates; API IP rate limits and four concurrent hash slots bound resource consumption; Nginx adds edge limits. |
| CSRF/origin | Bound the readable CSRF cookie/header to the stored session hash; blocked cross-origin mutations including login; required JSON POST bodies; added mandatory production APP_ORIGIN; preserved original Host in reverse proxies. |
| RBAC | Current role/active flag loaded from MySQL on every protected request; admin manages records/audit, operator tests, viewer reads. UI hides unauthorized controls; backend remains authoritative. |
| Proxmox credentials | AES-256-GCM with independent external key and fresh nonce; constructor now strictly rejects non-32-byte keys; secrets never appear in inventory, audit or JSON responses. |
| PVE connector | Retained TLS trust verification, custom CA support, CIDR-checked dialer, HTTPS port 8006, no environment proxy and no redirect following; bounded connections, reads and timeouts; validates nonempty version/node discovery and response structure/size. |
| Registration/tests/removal | Independent endpoint identity and credential lookup; initial failure retains registration; explicit test persists status; checked timestamp returned; duplicate conflicts distinguished from unavailable database/audit service. Removal affects only manager records. |
| Audit | Successful login/logout, registration/test/removal and CLI user creation are transactional with audit rows; failed passwords recorded without credentials; audit failures roll back associated state changes; admin UI lists recent events. |
| Provisioning | Email validation; concurrent bootstrap cannot create two admins; CLI supports operator/viewer only; CLI creation is audited. |
| Deployment | Production loopback/secure-cookie/origin enforcement; safe database startup logging; DB parseTime/UTC/timeouts enforced in code; nonzero API bind-failure exit; fixed Nginx security-header inheritance; corrected environment/CA permissions and least-privilege audit grants. |
| Angular usability | Explicit zone change detection for asynchronous subscriptions; scoped CSRF interceptor to same-origin API URLs; expired sessions route to login; viewer cannot see Test; list errors clear loading; failed logout is shown; successful login clears password; admin audit panel. |
| Dependencies | Updated golang.org/x/crypto to v0.59.0 and x/sys to v0.50.0; patched minimatch/glob overrides retain Angular 21 while removing development dependency advisories. |
| Documentation | Updated architecture, API notes, complete Ubuntu 24.04 runbook, authentication component/request-flow explanation, reproducible verification instructions and real-PVE acceptance script. |

## Findings addressed

| Finding | Consequence in original source | Resolution |
|---|---|---|
| Missing context import | Backend could not compile. | Corrected and all commands built. |
| Stateless logout | Captured JWT remained usable after sign-out. | Durable per-session lookup and deletion on logout. |
| Unbound double-submit CSRF | Cookie/header equality alone did not bind a CSRF token to the authenticated session. | Session-specific CSRF hash plus origin checks. |
| No login origin/body-type defense | HTML-form or cross-origin login handling lacked a complete application-level boundary. | JSON-only and same-origin checks, including login. |
| Ignored database/audit write errors | API could report success with missing audit/status data. | Transactional state/audit writes and explicit failure responses. |
| Login count/reset race | Concurrent requests could bypass the intended lockout ordering. | User row lock covering verification and update. |
| Concurrent admin bootstrap | Two bootstrap processes could both observe zero admins. | Database advisory lock, tested concurrently. |
| AES constructor accepted 128/192-bit keys | Security primitive did not enforce its advertised AES-256 contract. | Exactly 32 bytes required. |
| Incomplete PVE response bounds | Null/missing/trailing data or overlong upstream fields could be accepted or fail persistence unexpectedly. | Strict data checks, bounded reads/fields and explicit errors. |
| Missing serve target and proxy host rewrite | Local Angular startup failed or new origin validation rejected requests. | Added serve configuration and changeOrigin=false. |
| Viewer Test control and stuck loading | UI offered forbidden actions and could remain stuck after failure. | Role-aware template and error state cleanup. |
| Angular 21 change-detection assumptions | Async subscription changes risked not appearing under the framework default. | Explicit zone-based provider, browser-tested. |
| Nginx add_header inheritance | Location-level Cache-Control replaced parent security headers. | Removed those overrides; API supplies no-store and static expiry stays supported. |
| CA directory permissions | Service user could not traverse the original root-only CA directory. | root:mmp directory 0750, root-only environment 0600, CA 0640. |
| DSN-dependent parsing/timezone | DATETIME scanning and UTC behavior depended entirely on an operator-supplied DSN. | Enforced by database.Open, with timeouts. |
| Successful process exit on listen failure | systemd Restart=on-failure could miss a failed listener. | Nonzero process exit on listen failure. |

## Verification performed

Environment: Go 1.27.4 (official checksum verified), Node 24.22.1, Angular CLI 21.2.30 / framework 21.2.26, TypeScript 5.9.3, disposable MySQL 8.4.13, Chromium, and Nginx 1.24 syntax validation in a disposable container. Dependencies were installed through the environment's normal trusted networking.

| Check | Result / scope |
|---|---|
| All Go packages, race detector | PASS with TEST_MYSQL_DSN set; integration suite executed rather than skipped. |
| Go vet | PASS. |
| Three binaries | API, create-admin and create-user build successfully with make build. |
| Config/security tests | Production origin/bind/cookie/key/CIDR validation, password verification, ciphertext tampering/nonces/key sizes, session uniqueness/expiry/tampering/key/lifetime checks. |
| MySQL HTTP integration | Login cookies, unauthenticated access, forged/missing CSRF, admin/operator/viewer RBAC, role refresh, disabled account, encrypted database contents, no secret in response, two installations, duplicate rejection, persisted tests, audit read, logout replay rejection, session expiry, eight-failure lockout. |
| Audit fault injection | Renamed audit table temporarily in the disposable DB; deletion and registration fail and roll back instead of committing unaudited changes. |
| PVE TLS integration | Separate HTTPS fixtures on distinct addresses with distinct credentials and trusted test certificates; version/node discovery; cross-installation wrong secret rejected; untrusted TLS and outside-CIDR destination rejected. |
| PVE response tests | HTTP 401/403/302, malformed/null/missing/trailing/oversized JSON rejected; upstream response text is not leaked. |
| CLI concurrency/provisioning | Two simultaneous bootstrap commands produce exactly one admin; operator/viewer creation and audit verified; create-user refuses admin escalation. |
| Angular install/build | npm ci and npm run build PASS. Production initial bundle about 339 kB raw / 92 kB estimated transfer. |
| Chromium | 4/4 PASS: admin registration/failure display/audit/removal/logout/guard; operator test controls; viewer controls; failed list request recovery. Browser tests used the development server against the real Go API/MySQL. |
| Nginx syntax | nginx -t PASS against supplied virtual host with temporary test certificates, using Nginx 1.24. |
| npm audit | Zero vulnerabilities across runtime and development dependencies after patched overrides. |
| govulncheck v1.9.0 | Final scan reports no vulnerabilities. Initial scan identified 18 advisories in older required modules, none in imported/used packages; dependency upgrades removed those findings. |
| Live acceptance script | Python syntax compilation passed; script not run against real PVE installations. |

Coverage is not complete: command entrypoints were exercised manually rather than by unit coverage; database.Open is exercised through integration tests in another package. The final coverage run reported approximately 91.5% config, 75% HTTP API, 82.5% PVE client and 78.2% security. Untested failure branches and operations are not implied to pass. Exact verification output is in validation-results.txt.

## Remaining issues and work

1. **Required deployment acceptance:** install on Ubuntu Server 24.04, supply trusted PVE CA(s), narrow routes/CIDRs and distinct tokens, then run scripts/test-live.py against at least two genuinely independent PVE installations. Validate actual PVE versions, privilege-separated ACL behavior, DNS, firewall and certificate SANs.
2. **Account lifecycle:** no web user management, password change/reset/recovery, MFA or SSO. Local bootstrap/operator/viewer CLI provisioning is available. Administrative changes require a controlled local/DB procedure.
3. **Credential lifecycle:** no in-place token-edit/rotation UI, automated key rotation, KMS or per-record authenticated encryption context. Existing ciphertext is application-bound; a privileged database writer could transplant valid ciphertext between rows. Limit database access and keep backup/key custody separate. Removing/re-registering allows replacing a token, with new manager identity/history.
4. **Audit operations:** audit logs are local MySQL records; an off-host immutable sink, scheduled retention and tamper-evident chaining are absent. Runtime grants prevent updating/deleting audit rows, but a DBA still controls them. Unknown/inactive/locked-account failure auditing is best-effort during outages and logged on failure; successful auth/state mutation auditing is mandatory.
5. **Operational scale:** single API/database deployment, bounded in-process login limits, no distributed limiter, HA, polling scheduler, metrics or alerting. Sessions are shared through MySQL; limits are per API process.
6. **Phase 2+:** full VM/LXC/storage inventory and resource operations, jobs/task tracking, backup/snapshot/power controls remain deferred by the included architecture. None were removed from the original project.
7. **Tooling maintenance:** patched transitive glob/minimatch versions are explicit overrides; reassess/remove them when Angular 21's upstream dependency tree resolves the advisories. Scans are point-in-time and do not guarantee future vulnerability status.
8. **Delivery environment:** the configured /codex output directory is unavailable in this attached execution environment (filesystem permission failure; sudo is absent). Project source and the prepared archive/report are retained in /workspace/work for workspace retrieval. This is a handoff-path limitation, not a build/test failure.

## Installation and review starting points

- Ubuntu instructions: docs/phase-1-runbook.md.
- Architecture and Phase 1 boundary: docs/architecture.md.
- Authentication components/request flow/token handling: docs/authentication.md.
- Repeatable automated/live checks: docs/testing.md.
- Apply migrations/001_init.sql then migrations/002_sessions.sql before API startup.

Real credentials must be entered only on the target host/environment and in the HTTPS registration form. No production credentials were supplied, created or included in this delivery.
