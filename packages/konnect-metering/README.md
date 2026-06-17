# @pymthouse/konnect-metering

Typed client for [Konnect Metering & Billing v3](https://developer.konghq.com/konnect/metering-and-billing/). Used by clearinghouse (and future consumers) to provision meters, features, plans, customers, and subscriptions against Kong Konnect.

## Source of truth

Types are generated from Kong's hosted OpenAPI spec — no vendored YAML in this repo:

https://raw.githubusercontent.com/Kong/developer.konghq.com/main/api-specs/konnect/metering-and-billing/v3/openapi.yaml

The generated output is committed as [`src/schema.gen.ts`](src/schema.gen.ts). CI and local builds compile that file directly (no network fetch on `pnpm build`).

## Commands

From the repo root:

```bash
pnpm --filter @pymthouse/konnect-metering generate   # refresh schema.gen.ts from hosted spec
pnpm --filter @pymthouse/konnect-metering build
pnpm --filter @pymthouse/konnect-metering test
```

Or from this directory:

```bash
pnpm generate && pnpm build && pnpm test
```

## Regeneration policy

Re-run `pnpm generate` when Kong publishes API changes you want to adopt, then commit the updated `schema.gen.ts`. Review the diff for new endpoints or breaking type changes before merging.

## Usage

```typescript
import { createKonnectClient } from "@pymthouse/konnect-metering";

const client = createKonnectClient({
  baseUrl: "https://us.api.konghq.com/v3/openmeter", // normalized to …/v3
  apiKey: process.env.OPENMETER_API_KEY!,             // kpat_… PAT
});

await client.waitForHealthy();

const { data } = await client.api.GET("/openmeter/meters");
console.log(data?.data);
```

From environment variables:

```typescript
import { createKonnectClientFromEnv } from "@pymthouse/konnect-metering";

const client = createKonnectClientFromEnv(); // OPENMETER_URL + OPENMETER_API_KEY
```

## URL conventions

| Helper | Value |
|--------|--------|
| API client base (`normalizeKonnectMeteringUrl`) | `https://{region}.api.konghq.com/v3` |
| CloudEvents ingest (`konnectIngestUrl`) | `…/v3/openmeter/events` |

Pass `OPENMETER_URL` as either the v3 root or `…/v3/openmeter`; both normalize to the same client base.

## Hand-written surface

| File | Role |
|------|------|
| `client.ts` | `openapi-fetch` factory, bearer auth middleware, `waitForHealthy` |
| `query-serializer.ts` | Konnect `filter` / `page` deepObject query encoding |
| `url.ts` | Region defaults, URL normalization, `buildCustomerKey` |
| `errors.ts` | `KonnectApiError` for non-2xx responses |
| `schema.gen.ts` | Generated types (do not edit by hand) |
