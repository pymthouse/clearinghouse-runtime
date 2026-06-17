# Pending changes

List user-facing changes here as part of your PR. Cleared on release.

- Project scaffolding: TypeScript/pnpm package depending on `@pymthouse/builder-sdk@^0.4.1`, with build/lint/test wiring and CI.
- `@pymthouse/konnect-metering`: OpenAPI-generated typed client for Konnect Metering & Billing v3 (`packages/konnect-metering`).
- OpenMeter admin layer: port/adapter/factory pattern for Konnect and self-hosted backends (`src/admin/`), with catalog and customer provisioning services.
- Bootstrap CLI: `pnpm openmeter:bootstrap` and `pnpm provision:customer` scripts for Konnect catalog and per-customer subscriptions.
