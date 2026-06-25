# tenant-admin (registry draft)

HTTP API + CLI for per-tenant **billing `clientId`** and **Auth0 public `auth0ClientId`** (`azp`)
mapping. The identity webhook reads `data/apps.json` to build 3-part `auth_id`:

`tenantId:clientId:externalUserId`

## API (port 8093)

All `/admin/*` routes require `Authorization: Bearer $ADMIN_SECRET`.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/health` | Liveness |
| `GET` | `/admin/tenants` | List tenants + apps |
| `POST` | `/admin/tenants` | Provision tenant (+ optional app/auth0 link) |
| `POST` | `/admin/apps/auth0` | Link Auth0 public client to tenant billing app |
| `GET` | `/admin/tenants/{tenantId}` | Tenant detail + apps |

### Provision tenant

```bash
curl -sS -X POST http://localhost:8093/admin/tenants \
  -H "Authorization: Bearer $ADMIN_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "tenantId": "acme",
    "tenantName": "Acme Inc",
    "clientId": "app_acme",
    "auth0ClientId": "EmzHYs1Dhlad8fElc5OLQy28MCZnOoWI"
  }'
```

### Link Auth0 bootstrap client

After `auth0-livepeer` bootstrap, sync the public client id:

```bash
node tenant-admin/cli.mjs sync-auth0-bootstrap \
  --bootstrap-env ../auth0-livepeer/.env.livepeer \
  --tenant-id demo \
  --client-id demo-client \
  --tenant-name "Demo"
```

## CLI

```bash
node tenant-admin/cli.mjs provision-tenant \
  --tenant-id acme --tenant-name "Acme Inc" \
  --client-id app_acme --auth0-client-id <AUTH0_PUBLIC_CLIENT_ID>

node tenant-admin/cli.mjs list
```

## Registry shape (`data/apps.json`)

```json
[
  {
    "tenantId": "demo",
    "clientId": "demo-client",
    "auth0ClientId": "EmzHYs1Dhlad8fElc5OLQy28MCZnOoWI"
  }
]
```

`auth0ClientId` is the Auth0 **public** application id from bootstrap (`AUTH0_PUBLIC_CLIENT_ID`).
JWT `azp` must match this value for OIDC streaming auth.
