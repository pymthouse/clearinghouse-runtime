# tenant-admin

A thin provisioning **control plane** for Clearinghouse multi-tenancy. It is *not* a
usage or billing proxy: it creates and maintains the Auth0, Konnect, and OpenMeter
resources a tenant needs, then gets out of the data path. Integrators submit usage via
the collector and (eventually) read metering/billing data directly from OpenMeter with
their own scoped credentials.

## Responsibilities

For each tenant, `tenant-admin` ensures (idempotently):

| System    | Resource |
|-----------|----------|
| Auth0     | Organization, admin users, org connection, membership |
| Konnect   | Team, per-tenant `tenant-{id}-ingest` system account, `Ingest` role binding, team membership, SPAT lifecycle |
| OpenMeter | Global meter/feature catalog; per-identity customer + subscription |

### Customer key scheme

The OpenMeter customer key and event subject key are **`${clientId}:${externalUserId}`**,
identical to builder-sdk (`authIdFromIdentity`) and pymthouse (`buildOpenMeterCustomerKey`).
The collector emits this exact string as the CloudEvent `subject`, so usage attributes to
the customer ensured here. `tenantId` is control-plane metadata (Auth0 org / Konnect team)
and a customer label, but is **not** part of the customer key. Do not change this format
without changing builder-sdk and pymthouse in lockstep.

It keeps a **metadata-only** registry (`data/tenants.json`, `data/apps.json`). It never
persists SPAT values — token *values* are returned to the caller once at creation and
only the opaque token *ID* is stored so it can be revoked on rotation.

## Credential model

Three distinct identities, by purpose:

- **`clearinghouse-provisioner`** — long-lived SPAT held by this service
  (`OPENMETER_PROVISIONER_PAT`). Used for all OpenMeter catalog/customer/subscription
  operations. Never returned to integrators.
- **`tenant-{id}-ingest`** — per-tenant machine identity with the **`Ingest`** role
  only. Its SPAT is returned once to whoever drives event ingestion.
- **`tenant-{id}-reader`** *(not yet issued)* — intended to carry `Metering Viewer` +
  `Billing Viewer` for trusted integrators doing direct OpenMeter reads.

### ⚠️ Security gate: reader tokens are disabled

Reader-token issuance (`POST /v1/tenants/{id}/tokens` with `{"kind":"reader"}`) returns
**501 Not Implemented** by design. Konnect role assignment in this codebase uses
wildcard entity scope (`entityID = "*"`, `EntityRegion = Wildcard`, `EntityType =
Metering`) — see [`internal/konnect/systemaccounts.go`](internal/konnect/systemaccounts.go).
For the write-only `Ingest` role that is acceptable. A wildcard **read** scope, however,
would expose every tenant's metering/billing data.

Do **not** enable reader tokens until one of these is confirmed:

1. Konnect can scope `Metering`/`Billing` roles to a per-tenant resource boundary, **or**
2. each tenant gets its own Konnect/OpenMeter isolation boundary, **or**
3. reads go through a small proxy that enforces a `tenant_id` filter.

## API

### Admin API (`TENANT_ADMIN_LISTEN_ADDR`, default `:8093`)

All routes except `/health` require `Authorization: Bearer ${ADMIN_SECRET}`.

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/v1/tenants` | Provision Auth0 org + Konnect team + ingest account + initial app mapping + OpenMeter catalog. Returns metadata and the ingest SPAT **once**. |
| `GET`  | `/v1/tenants` | List tenant metadata. |
| `GET`  | `/v1/tenants/{tenantId}` | Tenant metadata + app mappings + token IDs (no token values). |
| `PUT`  | `/v1/tenants/{tenantId}/apps/{clientId}` | Register/update an Auth0 client → tenant mapping. |
| `POST` | `/v1/tenants/{tenantId}/tokens` | Body `{"kind":"ingest"}` → new ingest SPAT (returned once). `"reader"` → 501 (gated). |
| `POST` | `/v1/tenants/{tenantId}/tokens/{tokenId}/rotate` | Issue a new ingest SPAT and revoke the prior one. |

### Internal API (`TENANT_ADMIN_INTERNAL_LISTEN_ADDR`, default `127.0.0.1:8094`)

Bind to loopback or a private network only. If `INTERNAL_API_SECRET` is set, requests
must present a matching `X-Internal-Secret` header (defense in depth).

| Method | Path | Purpose |
|--------|------|---------|
| `POST` | `/internal/customers/ensure` | Ensure an OpenMeter customer + subscription. Body: `{ "clientId", "externalUserId" }` (or `{ "subject" }` / `{ "authId" }` = `clientId:externalUserId`, split on first colon). |

This is the proactive **"know the customer before service"** path: the identity webhook
calls it synchronously when it resolves an identity, before the signer is told to sign.
The customer is keyed `clientId:externalUserId`; `tenantId` is resolved from the registry
(by `clientId`) for labelling only. It does not re-ensure the catalog, so it stays cheap on
the hot path — the catalog is a provisioning-time concern.

There are intentionally **no** public `/usage` or `/balance` endpoints.

## Configuration

| Env var | Required | Default | Notes |
|---------|:--------:|---------|-------|
| `ADMIN_SECRET` | ✓ | — | Bearer token for the admin API. |
| `KONNECT_PLATFORM_PAT` | ✓ | — | Platform PAT used to manage teams/system accounts/roles/SPATs. |
| `OPENMETER_PROVISIONER_PAT` | ✓ | — | `clearinghouse-provisioner` SPAT for OpenMeter ops. |
| `AUTH0_MGMT_DOMAIN` | ✓ | — | Auth0 Management API domain. |
| `AUTH0_MGMT_CLIENT_ID` | ✓ | — | Auth0 management client. |
| `AUTH0_MGMT_CLIENT_SECRET` | ✓ | — | Auth0 management secret. |
| `TENANT_ADMIN_LISTEN_ADDR` | | `:8093` | Admin API listener. |
| `TENANT_ADMIN_INTERNAL_LISTEN_ADDR` | | `127.0.0.1:8094` | Internal API listener (loopback; `:8094` under compose). |
| `INTERNAL_API_SECRET` | | — | If set, internal API requires a matching `X-Internal-Secret` header. |
| `TENANT_ADMIN_DATA_DIR` | | `./data` | Registry storage. |
| `KONNECT_API_URL` | | `https://global.api.konghq.com` | |
| `KONNECT_INGEST_ROLE` | | `Ingest` | Role assigned to the tenant ingest account. |
| `KONNECT_SPAT_TTL_DAYS` | | `365` | SPAT lifetime. |
| `OPENMETER_URL` | | `https://us.api.konghq.com/v3/openmeter` | |
| `OPENMETER_DEFAULT_PLAN_KEY` | | `clearinghouse_default_ppu` | Plan key for subscriptions. |
| `AUTH0_DEFAULT_CONNECTION` | | `Username-Password-Authentication` | |

> **Role names:** current Kong Metering & Billing roles are `Ingest`, `Metering Admin`,
> `Metering Viewer`, `Product Catalog Admin`, `Billing Admin`, `Billing Viewer`. Use
> these — the older `Meter Admin` name is obsolete.

## CLI modes

```bash
# Run the control-plane servers (default)
tenant-admin -mode server

# One-shot tenant provisioning
tenant-admin -mode provision-tenant \
  -tenant-id acme -tenant-name "Acme" \
  -admin-emails admin@acme.com -admin-password 'Password123!' \
  -client-id app_acme -external-user-id bootstrap-user

# Ensure a single OpenMeter customer/subscription (keyed clientId:externalUserId)
tenant-admin -mode ensure-customer \
  -client-id app_acme -external-user-id user_1

# Add -dry-run to any mode to skip mutations.
```

## Development

```bash
go build ./...
go test ./...
```
