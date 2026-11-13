# Verification

Use Go 1.27.4, Node 24.22.1 (or a supported Angular 21 Node version), npm and a **disposable** MySQL 8 database. Never point the integration tests at production: `TestMySQLPhase1` clears users, sessions, servers and audit rows.

```bash
go test -race ./...
go vet ./...
make build
cd web
npm ci
npm run build
npm audit
```

The MySQL suite skips when `TEST_MYSQL_DSN` is unset. To exercise it:

```bash
export TEST_MYSQL_DSN='mmp_test:YOUR_TEST_PASSWORD@tcp(127.0.0.1:13306)/mmp_test'
go test -race -count=1 ./...
```

Create the database/user with permissions for creating tables and renaming the audit table, because the test deliberately simulates audit failure. The integration test applies migrations 001 and 002 itself. The TLS fixtures bind port 8006 on 127.0.0.2 through 127.0.0.5. These addresses must be available; do not run duplicate suites simultaneously against the same database or ports. Fixtures verify the actual CIDR-checked dialer, certificate validation and PVE authorization header. They simulate PVE responses; they are not hypervisors.

## Chromium acceptance

After the MySQL suite, it leaves admin/operator/viewer@example.com with password `integration-password-2026`, intentionally disables the viewer, demotes the operator, and expires sessions as security assertions. Restore **only in the disposable database** before browser testing:

```sql
UPDATE users SET is_active=TRUE, locked_until=NULL, failed_login_count=0,
 role=CASE WHEN email='operator@example.com' THEN 'operator' ELSE role END;
```

Start the API with the test database, a test signing key and `CREDENTIALS_ENCRYPTION_KEY` equal to base64 of 32 bytes of value 42 (`KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio=`). Set `APP_ENV=development`, `COOKIE_SECURE=false`, `APP_ORIGIN=` and `PVE_ALLOWED_CIDRS=127.0.0.0/8`. These are test-only settings. Start `cd web && npm start` separately. The test API fixture listeners need not remain running: the UI suite exercises the failure path on an existing saved endpoint.

```bash
cd web
npx playwright install chromium
E2E_PASSWORD=integration-password-2026 npm run test:e2e
# To use an existing browser: CHROMIUM_PATH=/usr/bin/chromium
```

The browser suite registers and removes a disposable record named `Browser acceptance`. Use an isolated database and manager. It verifies login/logout/route guarding, admin registration/removal/audit access, operator test controls, viewer restrictions and recovery from a failed list request. Do not run MySQL and browser suites concurrently.

## Real PVE acceptance

From the deployed manager's Ubuntu host, configure the CA bundle and narrow CIDR allowlist, then register 2–5 independent installations using distinct read-only PVE tokens. Run:

```bash
MMP_URL=https://manager.example.com MMP_EMAIL=admin@example.com python3 scripts/test-live.py
```

The script prompts for a local password, tests every registered endpoint, reports IDs/version/node counts without credentials, logs out in a finally block, and exits nonzero on failure. It performs no VM/cluster mutations. Each test updates manager connection status and audit logs. A count of two records cannot prove the hypervisors are independent: the operator must verify this topology. This live acceptance still requires actual reachable installations and has not been performed in this delivery.
