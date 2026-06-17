# Multi-tenant billing architecture

## Roles

| Component | Responsibility |
|-----------|----------------|
| **builder-sdk** | Webhook verify, identity contracts, `BillingProvisionerPort` / `UserProvisionerPort`, optional OpenMeter adapter |
| **clearinghouse compose** | Single-tenant runtime: global env, identity-webhook with injected provisioners |
| **pymthouse / platform app** | Tenant config DB, per-app credentials, admin APIs, plan policy |

## pymthouse reference (multi-tenant)

- **Tenant config:** `appOpenMeterConfig` table + `resolveAppOpenMeterConfig(clientId)` in `src/lib/openmeter/client-factory.ts`
- **Modes:** `pymthouse_hosted` (platform Konnect env) vs BYO OpenMeter (per-app `baseUrl` + encrypted API key)
- **User provision:** `POST /api/v1/apps/{id}/users` → `provisionAppUserBilling({ clientId, externalUserId })`
- **Usage reads:** `getOpenMeterClientForApp(clientId)` — not global env

## clearinghouse today (single-tenant)

`deploy/identity-webhook/server.mjs` wires:

```js
createOpenMeterBillingProvisioner({
  client: openMeterClient,
  resolvePlanKey: () => process.env.OPENMETER_DEFAULT_PLAN_KEY,
});
```

Platform `clientId` comes from JWT `azp` or `AUTH0_PUBLIC_CLIENT_ID` fallback.

## Migration path to multi-tenant clearinghouse

1. **Phase 1 (current):** Single-tenant compose; ports injected at process start.
2. **Phase 2:** Add clearinghouse app DB for per-tenant billing config (mirror pymthouse schema).
3. **Phase 3:** Implement `BillingProvisionerPort` in app layer that resolves client + plan per `clientId`.
4. **Phase 4:** Move `POST /admin/customers` to platform admin API; identity-webhook verify-only (`adminRoutes: false`).
5. **Phase 5:** Optional Konnect OpenAPI-generated client in builder-sdk (replace `@openmeter/sdk` shim).

## What not to put in builder-sdk

- `OPENMETER_URL` / API key persistence
- Per-tenant plan catalog in Postgres
- App ownership / authz for admin routes
