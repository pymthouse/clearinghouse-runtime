# Pending changes

List user-facing changes here as part of your PR. Cleared on release.

- Project scaffolding: TypeScript/pnpm package depending on `@pymthouse/builder-sdk@^0.4.1`, with build/lint/test wiring and CI.
- Docker deploy stack (`deploy/`): Kafka/Redpanda, go-livepeer remote signer (no Apache DMZ, CLI port bound to loopback only), and OpenMeter/Benthos collector; docker-compose for local dev.
- OpenMeter/Konnect bootstrap scripts (`scripts/lib/meters.ts`, `openmeter.ts`, `konnect-metering.ts`, `config/meters.json`, `pnpm openmeter:bootstrap`): provisions `network_fee_usd_micros`/`signed_ticket_count` meters and `network_spend` feature; Konnect-first, self-hosted OpenMeter fallback.
- Railway CI (`deploy/*/railway.json`, `scripts/railway-*.sh`, `.github/workflows/deploy-railway.yml`): per-service manifests + deploy scripts mirroring pymthouse; off by default (`RAILWAY_AUTO_DEPLOY` repo var), no hardcoded project ID.
