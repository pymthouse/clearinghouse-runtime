# Auth0 tenant provisioning (`auth0ctl`)

Idempotent provisioning of an Auth0 tenant slice — tenant device-flow settings, a
resource server (API), and one or more **public/confidential client pairs** — driven
by a declarative [`config/auth0.yaml`](config/auth0.yaml) against the [Auth0
Management API](https://auth0.com/docs/api/management/v2) via the official
[`go-auth0`](https://github.com/auth0/go-auth0) SDK.

It is the Auth0 counterpart of the OpenMeter/Konnect metering bootstrap in
[`../openmeter-collector/provision`](../openmeter-collector/provision): the catalog is
data (`config/auth0.yaml`); the reconcile is thin, idempotent, and safe to re-run.

## Why two clients per app

`auth0ctl` reproduces **pymthouse's OIDC issuer design** in Auth0. Each interactive app
is modeled as a sibling pair of OAuth 2.0 clients ([RFC 6749](https://www.rfc-editor.org/rfc/rfc6749)):

| Role | Auth0 `app_type` | `token_endpoint_auth_method` | Grant types | Used for |
| --- | --- | --- | --- | --- |
| **Public** | `native` | `none` (no secret) | `urn:ietf:params:oauth:grant-type:device_code`, `refresh_token` | SDK / CLI / browser, **device authorization** ([RFC 8628](https://www.rfc-editor.org/rfc/rfc8628)). Its grant scopes drive end-user token claims. |
| **Confidential (M2M)** | `non_interactive` | `client_secret_post` | `client_credentials` | Server-side Builder API and **token exchange** ([RFC 8693](https://www.rfc-editor.org/rfc/rfc8693)) for device-code binding. Its grant scopes gate server calls. |

Both clients are granted against one shared **resource server** (the API) whose
`identifier` is the JWT `aud`. The presence of `users:token` in the **public** client's
grant marks an app as per-user (per-user billing attribution); the **M2M** client carries
the server-side scopes (`users:write`, `users:token`, `device:approve`, and the
inherited `sign:job` / optional `sign:mint_user_token`).

## Prerequisites

- **Go 1.25+** (to build) or a prebuilt `auth0ctl` binary.
- An **Auth0 tenant**.
- An Auth0 **Machine-to-Machine application** authorized for the Management API, holding:
  `create:clients read:clients update:clients`,
  `create:resource_servers read:resource_servers update:resource_servers`,
  `create:client_grants read:client_grants update:client_grants`,
  `read:client_keys` (to read M2M secrets back on re-run), and
  `update:tenant_settings` (to enable device flow).

## Configuration (env)

`auth0ctl` auto-loads `.env` from the working directory when present (override with
`--env-file`). Existing shell variables take precedence over `.env` values.

| Variable | Purpose | Default |
| --- | --- | --- |
| `AUTH0_DOMAIN` | Tenant domain (e.g. `your-org.us.auth0.com`). | — (required) |
| `AUTH0_MGMT_CLIENT_ID` | Management API M2M client id. | — (required) |
| `AUTH0_MGMT_CLIENT_SECRET` | Management API M2M client secret. | — (required) |
| `AUTH0_CONFIG_PATH` | Declarative config path. | `config/auth0.yaml` |
| `BOOTSTRAP_OUTPUT` | Generated env output (contains secrets). | `.env.livepeer` |

One-time setup:

```bash
cp .env.example .env
# edit AUTH0_DOMAIN, AUTH0_MGMT_CLIENT_ID, AUTH0_MGMT_CLIENT_SECRET
```

## Usage

```bash
make build          # produces ./auth0ctl
./auth0ctl          # provision from config/auth0.yaml, write .env.livepeer

# Explicit paths:
./auth0ctl --config config/auth0.yaml --output .env.livepeer
```

`auth0ctl` is safe to re-run. It reconciles in dependency order — tenant settings →
resource servers → client pairs → client grants — and at each step **lists, then
creates-if-absent or updates-in-place**. It never blind-creates, so a second run logs
`exists`/`updated` rather than erroring on conflict, and never produces duplicate
clients.

## What it provisions

From [`config/auth0.yaml`](config/auth0.yaml):

| Kind | Identity | Notes |
| --- | --- | --- |
| Tenant settings | — | `default_audience`, `default_directory`, `device_flow` (charset/mask) — enables [RFC 8628](https://www.rfc-editor.org/rfc/rfc8628) device flow. |
| Resource server | `livepeer-clearinghouse` | API; `identifier` = audience; RS256; `allow_offline_access` for refresh tokens; full scope set (`sign:job`, `users:*`, `device:approve`, `admin`). |
| App pair | `Demo App` | Public (`Demo App Public`) + M2M (`Demo App M2M`); app-level. |
| App pair | `Per-User App` | Public carries `users:token` ⇒ per-user billing mode; M2M adds `sign:mint_user_token`. |

The generated `.env.livepeer` carries the shared tenant values (`AUTH0_DOMAIN`,
`AUTH0_ISSUER`, `AUTH0_JWKS_URL`), identity-webhook hints (`CLAIM_CLIENT_ID=azp`,
`USAGE_SUBJECT_TYPE=auth0_user_id`), and one block per pair
(`<APP>_AUTH0_PUBLIC_CLIENT_ID`, `<APP>_AUTH0_M2M_CLIENT_ID`,
`<APP>_AUTH0_M2M_CLIENT_SECRET`).

## Device authorization flow (RFC 8628)

For the OAuth 2.0 Device Authorization Grant to mint API access tokens, three things
must hold; `auth0ctl` configures all three:

1. The **public client** carries the `urn:ietf:params:oauth:grant-type:device_code` grant.
2. The **resource server** allows offline access (refresh tokens).
3. The **tenant** has a `default_audience`, so `/oauth/device/code` resolves an audience
   without the caller passing one explicitly.

End-to-end check against the provisioned tenant:

```bash
# 1. Request a device + user code (public client; no secret).
curl -s -X POST "https://$AUTH0_DOMAIN/oauth/device/code" \
  -d "client_id=$PUBLIC_CLIENT_ID" \
  -d "audience=livepeer-clearinghouse" \
  -d "scope=openid sign:job offline_access"
#   → { device_code, user_code, verification_uri, verification_uri_complete, interval }

# 2. Approve user_code in a browser at verification_uri_complete.

# 3. Poll the token endpoint until approval completes.
curl -s -X POST "https://$AUTH0_DOMAIN/oauth/token" \
  -d "grant_type=urn:ietf:params:oauth:grant-type:device_code" \
  -d "device_code=$DEVICE_CODE" \
  -d "client_id=$PUBLIC_CLIENT_ID"
#   → access_token with aud=livepeer-clearinghouse, azp=$PUBLIC_CLIENT_ID
```

> **Optional — third-party initiate (pymthouse "Option B").** Set
> `apps[].public.initiateLoginUri` to have Auth0 redirect device verification to an
> integrator login endpoint before binding the grant. Server-side binding then uses
> RFC 8693 token exchange; see [pymthouse-integrations](../../pymthouse) for the
> `resource=urn:pmth:device_code:<user_code>` exchange contract.

## Identity contract

Tokens issued to the **public** client carry `azp` (and `client_id`) equal to the public
client id; the identity webhook reads `CLAIM_CLIENT_ID=azp` for attribution. The token
`aud` equals the resource server `identifier`. Per-user billing is selected by including
`users:token` in the **public** client's grant scopes — a server-only scope set on the
M2M client does not change end-user token claims.

## Key design decisions & trade-offs

- **Idiomatic `go-auth0` (v1) SDK, not the Auth0 Deploy CLI (`tenant.yaml`).** Keeps the
  toolchain Go, consistent with the repo's go-konnect provisioning, and produces a single
  static binary with no Node runtime. v1's plain pointer-field structs and `auth0.String/
  Bool/Int` helpers keep the reconcile readable. The trade-off is that we implement the
  loop ourselves rather than inheriting `a0deploy`'s drift detection.
- **Match on stable natural keys**, not stored ids: resource servers by `identifier`,
  clients by `name`, client grants by `(client_id, audience)`. This makes the config the
  source of truth without a local state file.
- **Server-side list filters where the API supports them.** Resource servers are fetched
  with an `identifier` filter and client grants with `client_id` + `audience` filters, so
  those lookups are O(1) requests. Only the client lookup paginates a bounded scan,
  because Auth0's clients endpoint has no name filter.
- **One `ensureClient` helper for both roles.** The public (native, device-flow) and
  confidential (M2M) clients differ only by a small `clientSpec` (app type, auth method,
  grant types, first-party, callbacks, initiate URI), so a single create-or-update path
  serves both — and because v1 models `token_endpoint_auth_method` as a plain string, it
  is reconciled uniformly on create and update.
- **M2M secrets are read back on re-run** via `read:client_keys` rather than rotated, so
  re-running does not invalidate already-distributed credentials.

## Limitations

- Client lookup paginates a bounded scan of all clients and matches by name (the clients
  endpoint has no server-side name filter); resource-server and client-grant lookups use
  server-side filters. For very large tenants, tag clients with `client_metadata` and
  filter on it instead.
- The provisioner creates and updates but does not **prune**: removing an app from the
  config does not delete its Auth0 clients (deletion is intentionally out of scope).
- Connections/users are not provisioned; `tenant.defaultDirectory` must reference an
  existing database connection (default `Username-Password-Authentication`).

## Implementation tasks

- [x] Go module scaffold (`go.mod`, `cmd/auth0ctl`, `internal/{config,auth0,output}`).
- [x] Declarative YAML schema + loader with validation (`internal/config/catalog.go`).
- [x] Idempotent reconcile: tenant settings, resource servers, client pairs, client
      grants (`internal/auth0/provision.go`).
- [x] Per-pair `.env.livepeer` output writer (`internal/output/env.go`).
- [x] Unit tests for config/catalog parsing and validation.
- [ ] Run against a real Auth0 tenant and verify the RFC 8628 flow end-to-end.
- [ ] (Optional) Server-side list filtering for large tenants.
- [ ] (Optional) `--prune` to remove clients/grants no longer in config.
- [ ] (Optional) Wire the `initiate_login_uri` Option-B device path to the pymthouse
      RFC 8693 exchange and document the integrator contract.
