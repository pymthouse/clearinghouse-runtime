# tenant-admin

Go service/CLI that provisions per-tenant Auth0 + Konnect + OpenMeter resources.

## What it provisions

- Auth0 organization per tenant with tenant-scoped admin users.
- Konnect team per tenant.
- Konnect system account + SPAT per tenant.
- Metering/billing roles on the system account: `Ingest`, `Meter Admin`, `Product Catalog Admin`, `Billing Admin` (metering-scoped wildcard entity).
- Optional OpenMeter customer + subscription bootstrap using tenant SPAT.

## Ingest scope spike findings

The Konnect API surface used here supports per-system-account role assignment and team membership (`/v3/system-accounts/*`, `/v3/teams/*`). The collector/plugin still authenticates to OpenMeter ingest with a token that has the `Ingest` role.

Two important constraints:

1. Konnect SaaS is not Kong Gateway Enterprise Workspaces. Tenant isolation is modeled with Konnect teams + token identity, not Gateway workspace entity partitioning.
2. Metering ingress authorization is role/token-based. Hard customer partitioning is enforced by tenant SPATs and a surrogate customer key derived from `{tenantId,clientId,externalUserId}`.

## Identity model

- `auth_id` in signer/kafka events is `tenantId:clientId:externalUserId` (3-part).
- OpenMeter customer key uses a deterministic surrogate: `ch_<sha256/auth tuple>`.
- Customer labels include `tenant_id`, `client_id`, `external_user_id` for debugging and filtering.
- This keeps keys far below Konnect's `ExternalResourceKey` limit (256 chars).

## Environment variables

- `ADMIN_SECRET` (required) bearer token for `/admin/*`.
- `TENANT_ADMIN_LISTEN_ADDR` (default `:8093`)
- `TENANT_ADMIN_DATA_DIR` (default `./data`)
- `KONNECT_API_URL` (default `https://global.api.konghq.com`)
- `KONNECT_PLATFORM_PAT` (required) platform bootstrap PAT/SPAT.
- `KONNECT_INGEST_ROLE` (default `Ingest`)
- `KONNECT_SPAT_TTL_DAYS` (default `365`)
- `OPENMETER_URL` (default `https://us.api.konghq.com/v3/openmeter`)
- `OPENMETER_DEFAULT_PLAN_KEY` (default `clearinghouse_default_ppu`)
- `AUTH0_MGMT_DOMAIN` (required)
- `AUTH0_MGMT_CLIENT_ID` (required)
- `AUTH0_MGMT_CLIENT_SECRET` (required)
- `AUTH0_DEFAULT_CONNECTION` (default `Username-Password-Authentication`)

## Modes

### Server

```bash
go run ./cmd/tenant-admin -mode server
```

### Provision tenant

```bash
go run ./cmd/tenant-admin -mode provision-tenant \
  -tenant-id acme \
  -tenant-name "Acme Inc" \
  -admin-emails "admin@acme.com" \
  -admin-password "ChangeMe123!"
```

### Migrate/create surrogate customer keys

```bash
go run ./cmd/tenant-admin -mode migrate-customer-keys \
  -tenant-id acme \
  -client-id app_acme \
  -external-user-id user_1
```

## API

- `GET /health`
- `GET /admin/tenants`
- `POST /admin/tenants`
- `GET /admin/tenants/{tenantId}`
- `GET /admin/tenants/{tenantId}/usage?clientId=...&externalUserId=...`
- `GET /admin/tenants/{tenantId}/usage/balance?clientId=...&externalUserId=...`
- `POST /admin/tenants/{tenantId}/spat/rotate`
- `POST /admin/customers`

All `/admin/*` routes require `Authorization: Bearer $ADMIN_SECRET`.

### Subscription status behavior

If customer creation succeeds but subscription creation is blocked by missing billing app setup (for example, Stripe customer data preconditions), tenant-admin returns success with:

- `status: "pending_billing_setup"`
- `subscriptionId: ""`

This keeps tenant provisioning idempotent while signaling that billing backend setup must be completed before subscriptions can be activated.
