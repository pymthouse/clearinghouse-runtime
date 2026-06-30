# Clearinghouse Builder API

Go HTTP service co-located in the `openmeter-collector` container. Provisions **Auth0 end-users**, **OpenMeter customers**, and mints **signer session JWTs** via Auth0.

Scalar docs: `GET /api/v1/docs` (spec at `/api/v1/openapi.json`).

## Endpoints

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| `POST` | `/api/v1/apps/{clientId}/users` | M2M Basic | Create/upsert Auth0 user + OpenMeter customer; returns `apiKey` once |
| `POST` | `/api/v1/apps/{clientId}/auth/api-key/signer-session` | Bearer `sk_…` | Exchange API key for short-lived signer JWT + upsert customer |
| `POST` | `/api/v1/apps/{clientId}/auth/oidc/signer-session` | Bearer Auth0 user JWT | Exchange device/OIDC token for signer JWT + upsert OpenMeter customer |

## Auth0 prerequisites

Most credentials are written to `auth0-provisioner/provision/.env.livepeer` by `./bootstrap.sh` and mounted into the collector at `/service/.env.livepeer`.

### 1. Management API M2M application

`bootstrap.sh` creates **Clearinghouse Builder Management** (M2M) with Management API scopes `create:users`, `read:users`, `update:users`, and writes:

```bash
AUTH0_MGMT_CLIENT_ID=...
AUTH0_MGMT_CLIENT_SECRET=...
```

Re-run `./auth0-provisioner/provision/bootstrap.sh` if these are missing from `.env.livepeer`. Set `managementClient.enabled: false` in `apps.json` to skip.

Tenant domain, issuer, audience, and signer M2M credentials come from the same file. The entrypoint maps `DEMO_APP_AUTH0_M2M_*` → `AUTH0_SIGNER_M2M_*` automatically.

### 2. Database connection

Enable a Database connection (default: `Username-Password-Authentication`) for end-user records. Set `AUTH0_DB_CONNECTION` if you use a different connection name.

### 3. Credentials-exchange Action (`external_user_id` + `client_id` claims)

Signer-token mint uses M2M `client_credentials` with `scope=sign:mint_user_token`, form fields `external_user_id`, and `client_id` (the **public** app client id from the Builder API path). Auth0 does not pass custom form fields into access tokens unless an **Action** adds them.

**Deploy manually** in the Auth0 dashboard (Actions → Library → Build Custom → Credentials Exchange trigger):

```javascript
exports.onExecuteCredentialsExchange = async (event, api) => {
  const externalUserId = event.request?.body?.external_user_id;
  const clientId = event.request?.body?.client_id;
  if (!externalUserId || !clientId) {
    return;
  }
  api.accessToken.setCustomClaim("external_user_id", externalUserId);
  api.accessToken.setCustomClaim("app_client_id", clientId);
};
```

1. Deploy the Action.
2. Bind it to the **Credentials Exchange** flow for your tenant.
3. Re-test signer-session — the JWT must include `external_user_id` and `app_client_id` for [identity-webhook](../../identity-webhook) OIDC verification (`OIDC_SUBJECT_CLAIM=external_user_id`, `OIDC_CLIENT_CLAIM=app_client_id`). Auth0 rejects the reserved claim name `client_id`; use `app_client_id` instead.

Without this Action, minted tokens verify at Auth0 but lack identity claims and the webhook rejects them.

### 4. Signer M2M (from bootstrap)

Provided automatically via mounted `.env.livepeer` (`DEMO_APP_AUTH0_M2M_CLIENT_ID` /
`DEMO_APP_AUTH0_M2M_CLIENT_SECRET`). Override in `openmeter-collector/.env` only if needed.

## Example: create user

```bash
set -a; source openmeter-collector/.env; set +a
CLIENT_ID="$AUTH0_SIGNER_M2M_CLIENT_ID"   # or your public client id path param
curl -sS -u "$AUTH0_SIGNER_M2M_CLIENT_ID:$AUTH0_SIGNER_M2M_CLIENT_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"externalUserId":"user-123","email":"user@example.com"}' \
  "http://localhost:8095/api/v1/apps/${DEMO_APP_AUTH0_PUBLIC_CLIENT_ID}/users"
```

Use the public client id from `.env.livepeer` as the `{clientId}` path segment (e.g. `DEMO_APP_AUTH0_PUBLIC_CLIENT_ID`).

## Example: signer session

```bash
API_KEY=sk_...   # from create-user response
curl -sS -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"scope":"sign:job"}' \
  "http://localhost:8095/api/v1/apps/${DEMO_APP_AUTH0_PUBLIC_CLIENT_ID}/auth/api-key/signer-session"
```

## Example: OIDC signer session (device code)

After device login, exchange the Auth0 user access token to provision OpenMeter and mint a signer JWT:

```bash
OIDC_TOKEN=...   # access_token from device code flow
curl -sS -H "Authorization: Bearer $OIDC_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"scope":"sign:job"}' \
  "http://localhost:8095/api/v1/apps/${DEMO_APP_AUTH0_PUBLIC_CLIENT_ID}/auth/oidc/signer-session"
```

The OpenMeter customer key is `{clientId}:{sub}` (e.g. `pub:google-oauth2|…`), matching the CloudEvent `subject`.

## OpenMeter customer key

Customers are upserted with:

- `key`: `{clientId}:{externalUserId}`
- `usage_attribution.subject_keys`: `["{clientId}:{externalUserId}"]`

This matches the collector CloudEvent `subject` / `auth_id` contract.

## Local development

```bash
cd openmeter-collector/builder-api
go run ./cmd/builder-api
```

Requires the same env vars as the container (`openmeter-collector/.env`).
