# Phase 1 API reference

Base path: `/api/v1`. All responses use JSON. All protected mutation requests require the `mmp_session` HttpOnly cookie and `X-CSRF-Token` header matching the `mmp_csrf` cookie.

## Register Proxmox endpoint

`POST /proxmox/servers` (admin only)

```json
{
  "name": "PVE-MAIN",
  "base_url": "https://172.30.0.10:8006",
  "token_id": "mmp-api@pve!mmp-controller",
  "token_secret": "one-time-token-secret",
  "location": "Main office",
  "notes": "Primary virtualization cluster"
}
```

The response contains the sanitized installation record and a connection result. It does not return `token_secret` or its encrypted representation.

## Test connection

`POST /proxmox/servers/{id}/test` (`admin` or `operator`)

On success, the result contains the Proxmox API version, release string, and a node list with available node status and basic resource fields. A failure returns HTTP 502 and a safe error description; the installation record remains registered.

## List installations

`GET /proxmox/servers` (any signed-in role)

Returns an `items` array. The API intentionally excludes `token_secret_enc`.


## Recent audit events

`GET /audit-logs?limit=100` (admin only)

Returns the latest audit events with actor, operation, target, source IP, structured details, and timestamp. The limit must be between 1 and 200.

## Session and CSRF behavior

Login accepts only application/json. Browser mutations with a cross-origin Origin are rejected, including login; production APP_ORIGIN must match the public HTTPS origin. Authenticated mutations also require X-CSRF-Token equal to mmp_csrf and its session hash. Cookies are host-only, SameSite=Strict; mmp_session is HttpOnly, and both cookies are Secure in production. Login does not return the signed session in JSON.

Migration 002 is mandatory. Logout revokes the database session and clears cookies; reusing the cookie returns 401. Each request reloads current database role/active status. There is no refresh token or silent refresh.

Login is limited to 10 requests per source IP per minute within each API process, with at most four concurrent password verifications, and eight failed passwords lock an account for 15 minutes. Nginx adds a stricter edge limit. A 429 carries Retry-After. A 503 indicates an unavailable authentication/audit/database transaction. Login/logout and state mutations do not report success if their required audit event cannot commit.

Registration persists before its initial network test. If the initial result cannot be saved, the API returns 503 with an explicit retained-registration message: reload the inventory and retry test rather than registering a duplicate. Failed network tests return 502 from the explicit test endpoint, with safe error text. No Proxmox token secret or raw upstream response body is returned. An empty node list is treated as failed permission/discovery validation.
