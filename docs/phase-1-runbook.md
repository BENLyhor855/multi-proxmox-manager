# Ubuntu Server 24.04 installation and operations

Use a dedicated Ubuntu Server 24.04 VM, separate from the Proxmox hypervisor. Phase 1 is a read-only connection control plane: it never powers off, deletes or modifies PVE resources. Build and test were performed on Debian 13 with Go 1.27.4, Node 24.22.1 and MySQL 8.4.13; the Ubuntu procedure below has not been executed on an Ubuntu VM during this delivery.

## 1. Network and packages

Permit inbound HTTPS only from the administrator network/VPN, SSH only from your administration network, and outbound TCP 8006 to each PVE management address. MySQL and the Go API must remain on loopback. Configure your firewall for SSH before enabling it, to avoid losing access.

```bash
sudo apt update
sudo apt install -y nginx mysql-server ca-certificates curl unzip make openssl python3 xz-utils
sudo systemctl enable --now mysql nginx
```

Ubuntu 24.04's `mysql-server` supplies MySQL 8.0, which supports this schema. MySQL 8.4 is also supported and is the version exercised by the integration tests. If using Oracle's 8.4 repository, follow Oracle's current repository installation instructions instead of mixing database package sources. Do not substitute MariaDB: the schema uses MySQL's `utf8mb4_0900_ai_ci` collation.

Install Go 1.27.4 and a Node version supported by Angular 21 (the verified Node version is 24.22.1). Example for amd64/x64; adapt architecture on ARM:

```bash
mkdir -p ~/mmp-build-tools
cd ~/mmp-build-tools
curl -fSLO https://go.dev/dl/go1.27.4.linux-amd64.tar.gz
printf '%s  %s\n' 7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840 go1.27.4.linux-amd64.tar.gz | sha256sum -c -
tar -xzf go1.27.4.linux-amd64.tar.gz
curl -fSLO https://nodejs.org/dist/v24.22.1/node-v24.22.1-linux-x64.tar.xz
curl -fSLO https://nodejs.org/dist/v24.22.1/SHASUMS256.txt
awk '$2 == "node-v24.22.1-linux-x64.tar.xz"' SHASUMS256.txt | sha256sum -c -
tar -xJf node-v24.22.1-linux-x64.tar.xz
export PATH="$HOME/mmp-build-tools/go/bin:$HOME/mmp-build-tools/node-v24.22.1-linux-x64/bin:$PATH"
go version
node --version
npm --version
```

Keep the tools on PATH for subsequent builds. Build as your ordinary account, then install the resulting binaries as root. No Go/Node toolchain is needed by the running systemd service.

## 2. Build the supplied source

Extract the completed project archive and enter its `multi-proxmox-manager` directory:

```bash
go mod download
go test ./...
go vet ./...
make build
cd web
npm ci
npm run build
npm audit
cd ..
```

Use committed `go.sum` and `web/package-lock.json`; do not regenerate the lockfile on production. `make build` disables VCS stamping so an extracted archive builds without a Git checkout. Full MySQL and browser test setup is in [testing.md](testing.md).

## 3. Create database, migrations and least-privilege grants

Use `sudo mysql` to create the schema and application user. Replace the placeholder with a unique strong password. A long hexadecimal password avoids shell/DSN punctuation escaping issues.

```sql
CREATE DATABASE multi_proxmox_manager CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;
CREATE USER 'mmp_app'@'127.0.0.1' IDENTIFIED BY 'REPLACE_WITH_LONG_RANDOM_PASSWORD';
```

Apply schema as the database administrator first:

```bash
sudo mysql multi_proxmox_manager < migrations/001_init.sql
sudo mysql multi_proxmox_manager < migrations/002_sessions.sql
```

After migrations, grant table-level permissions:

```sql
GRANT SELECT, INSERT, UPDATE ON multi_proxmox_manager.users TO 'mmp_app'@'127.0.0.1';
GRANT SELECT, INSERT, UPDATE, DELETE ON multi_proxmox_manager.sessions TO 'mmp_app'@'127.0.0.1';
GRANT SELECT, INSERT, UPDATE, DELETE ON multi_proxmox_manager.proxmox_servers TO 'mmp_app'@'127.0.0.1';
GRANT SELECT, INSERT ON multi_proxmox_manager.audit_logs TO 'mmp_app'@'127.0.0.1';
```

The application identity cannot update/delete audit rows or alter schema. Use a separate DBA identity for migrations, retention and restores. Existing installations must apply **002** before starting the new API; prior stateless sessions require sign-in again. Migrations are not automatically run at API startup. Inspect and back up before schema changes.

## 4. Install files and configure the service

From the project directory:

```bash
sudo useradd --system --home /opt/multi-proxmox-manager --shell /usr/sbin/nologin mmp
sudo install -d -o root -g mmp -m 0750 /opt/multi-proxmox-manager /opt/multi-proxmox-manager/bin
sudo install -o root -g mmp -m 0750 bin/mmp-api bin/create-admin bin/create-user /opt/multi-proxmox-manager/bin/
sudo install -d -o root -g mmp -m 0750 /etc/multi-proxmox-manager
sudo install -o root -g root -m 0600 .env.example /etc/multi-proxmox-manager/mmp.env
sudoedit /etc/multi-proxmox-manager/mmp.env
```

Set:

- `APP_ENV=production`, `APP_ORIGIN=https://your-manager-hostname`, `HTTP_ADDR=127.0.0.1:8080`, `COOKIE_SECURE=true`.
- `MYSQL_DSN` with the local database/user and unique password; preserve the quoted template format.
- `JWT_SECRET` from `openssl rand -hex 48`.
- `CREDENTIALS_ENCRYPTION_KEY` from `openssl rand -base64 32`, generated independently.
- `PVE_ALLOWED_CIDRS` with exact management host `/32` or `/128` ranges where practical. Avoid broad RFC1918 ranges.
- `PVE_CA_FILE` with a readable bundle containing each PVE issuing CA; leave empty only if certificates are already trusted by the system.

Never paste actual secrets into source files or the Angular build. The root-owned 0600 environment file is read by systemd before it drops privileges. The directory is root:mmp 0750 so the API can read its CA bundle:

```bash
sudo install -o root -g mmp -m 0640 /path/to/verified-pve-ca-bundle.pem /etc/multi-proxmox-manager/pve-ca-bundle.pem
```

For PVE certificates, the hostname/IP used in the API URL must match the certificate SAN. Obtain the CA through a trusted channel; do not trust a certificate merely because it was downloaded from an unverified connection.

## 5. Bootstrap local accounts

Run in a protected root shell; no secret appears in process arguments or shell history. The binaries were built as your ordinary user above.

```bash
sudo -i
set -a
source /etc/multi-proxmox-manager/mmp.env
set +a
export BOOTSTRAP_ADMIN_EMAIL=admin@example.com
read -rsp 'Initial admin password: ' BOOTSTRAP_ADMIN_PASSWORD; echo
export BOOTSTRAP_ADMIN_PASSWORD
/opt/multi-proxmox-manager/bin/create-admin
unset BOOTSTRAP_ADMIN_PASSWORD

export NEW_USER_EMAIL=operator@example.com NEW_USER_ROLE=operator
read -rsp 'Operator password: ' NEW_USER_PASSWORD; echo
export NEW_USER_PASSWORD
/opt/multi-proxmox-manager/bin/create-user
unset NEW_USER_PASSWORD
# Repeat with a unique viewer email and NEW_USER_ROLE=viewer as needed.
exit
```

Passwords must contain 14–1024 bytes. The first-admin command uses a database advisory lock and refuses subsequent administrators. CLI creation and its audit record commit together. There are no default passwords or public registration endpoints. Users/roles can be changed by a privileged DBA during Phase 1; use a controlled procedure, never insert plaintext password hashes. Self-service password changes and account recovery are not implemented.

## 6. Start API

```bash
sudo install -o root -g root -m 0644 deploy/systemd/mmp-api.service /etc/systemd/system/mmp-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now mmp-api
sudo systemctl status mmp-api --no-pager
curl -fsS http://127.0.0.1:8080/api/v1/healthz
```

Expected response: `{"status":"ok"}`. Logs: `sudo journalctl -u mmp-api -f`. Startup rejects a non-loopback production bind, insecure cookies, invalid origin, missing allowlist and invalid keys. Never log the environment file or DSN when troubleshooting.

## 7. Publish Angular with HTTPS Nginx

```bash
sudo install -d -o root -g www-data -m 0755 /var/www/multi-proxmox-manager
sudo cp -a web/dist/multi-proxmox-manager-web/browser/. /var/www/multi-proxmox-manager/
sudo chown -R root:www-data /var/www/multi-proxmox-manager
sudo find /var/www/multi-proxmox-manager -type d -exec chmod 0755 {} +
sudo find /var/www/multi-proxmox-manager -type f -exec chmod 0644 {} +
```

Edit `deploy/nginx/mmp.conf` to match `APP_ORIGIN`'s hostname and your trusted TLS certificate paths. Acquire the certificate first; for a public hostname, use a temporary HTTP-only Nginx virtual host and `certbot certonly --nginx -d your-manager-hostname` after installing `certbot python3-certbot-nginx`. An internal CA certificate is also appropriate for private deployments if browsers trust it.

```bash
sudo install -o root -g root -m 0644 deploy/nginx/mmp.conf /etc/nginx/sites-available/mmp.conf
sudo ln -s /etc/nginx/sites-available/mmp.conf /etc/nginx/sites-enabled/mmp.conf
sudo nginx -t
sudo systemctl reload nginx
```

The file includes `limit_req_zone` at the HTTP configuration level (Ubuntu includes `sites-enabled/*` in its `http` block). Preserve the original Host header: origin checks include the port when present. Avoid adding location-level `add_header` directives without repeating inherited security headers. Restrict access via firewall/VPN; neither 8080 nor 3306 should be externally reachable.

## 8. Provision each independent Proxmox installation

On **each** real PVE installation, create a distinct API identity/token using a PVE administrator:

```bash
pveum user add mmp-api@pve --comment 'Multi-Proxmox Manager integration'
pveum user token add mmp-api@pve mmp-controller --privsep 1
pveum acl modify / -user mmp-api@pve -role PVEAuditor
pveum acl modify / -token 'mmp-api@pve!mmp-controller' -role PVEAuditor
```

Both ACLs are required because a privilege-separated token has the intersection of the user/token permissions. Keep the one-time secret in a password manager until registered. Do not use root@pam or Administrator ACLs.

Sign in over HTTPS, select **Add installation**, and enter a distinct name, `https://hostname-matching-cert:8006`, `mmp-api@pve!mmp-controller` and its secret. Register 2–5 independent installations. A PVE cluster normally needs one cluster endpoint record, not one record for every cluster node. Registration persists even when the initial connection fails so operators can repair networking/CA trust and retry **Test**. Removing a record only removes manager data.

## 9. Acceptance and maintenance

Run `scripts/test-live.py` as described in [testing.md](testing.md), and record results from two or more genuinely independent installations. Verify admin/operator/viewer access, audit visibility, logout replay rejection, disallowed CIDR failure, invalid-token failure and certificate failure. Test the deployed HTTPS UI and verify firewall boundaries.

Back up MySQL consistently using a DBA/backup account and a protected defaults file (avoid passwords on the command line):

```bash
mysqldump --defaults-extra-file=/root/.mmp-backup.cnf --single-transaction --routines --triggers multi_proxmox_manager > /protected-backups/mmp.sql
```

Back up the encryption key separately with restricted access. Test restoration into an isolated database: install schema, restore data, use the original encryption key, clear restored `sessions` using a DBA identity, then verify a PVE test. Losing the encryption key requires replacing the stored API tokens; no recovery backdoor exists. Rotate the signing key to invalidate all JWTs, or delete session rows to revoke individual/all sessions. After a database restore, clear sessions before allowing logins.

Monitor audit-write errors, storage capacity, TLS expiry and PVE token permissions. State changes/login/logout fail if their audit transaction cannot commit; unknown/inactive/locked-account failure audit writes are best-effort and emit a server error log if unavailable. Retention is a DBA task; archive audit rows off-host before pruning according to your policy. No immutable remote sink or automatic encryption-key rotation is included in Phase 1.

## Troubleshooting

- **Login HTTP 403:** confirm `APP_ORIGIN`, browser URL and preserved Nginx Host match; use HTTPS in production. Development uses `APP_ENV=development`, empty `APP_ORIGIN`, `COOKIE_SECURE=false` and the supplied proxy with `changeOrigin=false`.
- **Login HTTP 429:** respect Retry-After and Nginx's per-IP limit. Eight wrong passwords lock an account for 15 minutes; do not repeatedly retry.
- **Login/state change HTTP 503:** check database/migrations and audit INSERT permissions. Do not disable auditing to work around it.
- **PVE TLS failure:** install the correct CA and use a hostname/IP matching SAN; never disable TLS verification.
- **PVE HTTP 401/403:** verify token ID/secret and both read-only ACLs.
- **PVE outside allowlist or timeout:** check routing, firewall and exact management CIDRs. Redirects and proxy environment variables are intentionally not used by the PVE client.
- **Registration response says status was not saved:** the registration is retained; refresh the inventory before retrying a test to avoid duplicate registration.
