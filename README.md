# clearinghouse

**A focused, official example of solving the remote-signer auth + usage-metering
problem for [go-livepeer](https://github.com/livepeer/go-livepeer) with
[`@pymthouse/builder-sdk`](https://github.com/pymthouse/builder-sdk).**

A go-livepeer orchestrator/gateway operator needs two things from a platform:

1. **Identity** — an `-remoteSignerWebhookUrl` endpoint that validates an
   end-user credential and returns the `UsageIdentity` / `expiry` go-livepeer
   needs to authorize a signing session.
2. **Metering** — usage emitted from a signed ticket session needs to land in a
   billing system, scoped to that end user, so balances and entitlements draw
   down correctly.

`@pymthouse/builder-sdk` already implements both: the remote-signer identity
webhook (OIDC / API-key / custom verifier adapters), per-tenant M2M token
minting, the direct-DMZ signer proxy, and OpenMeter-backed usage/allowance
helpers. **This repo is the reference application that wires those pieces
together** — the thing a platform operator clones, configures, and runs.

## Status

The **first MVP deliverable** is the Docker deploy stack in [`deploy/`](deploy/)
— Kafka/Redpanda, a go-livepeer remote signer (no Apache DMZ, CLI port not
exposed), the OpenMeter/Benthos collector, and OpenMeter/Konnect bootstrap
scripts. Railway CI is wired up and ships with the stack. See
[deploy/README.md](deploy/README.md) for local and Railway usage.

The TS/pnpm package scaffold, build, lint, test, and CI are also in place
(`@pymthouse/builder-sdk` resolves), but the `/authorize` webhook route,
OIDC/API-key adapters, and admin API land in follow-up issues. See the [issue
tracker](https://github.com/livepeer/clearinghouse/issues) and milestones for
the full roadmap.

## Packaging: one configurable package

**Decision:** `clearinghouse` ships as a **single npm package with
configuration-driven hosted/on-prem modes**, not as two separate packages.

Rationale:

- The core surface — the `/authorize` webhook handler, OIDC/API-key/custom
  verifier adapters, signer-proxy wiring, and the OpenMeter usage/allowance
  helpers — is **identical** in both modes. It's all `@pymthouse/builder-sdk`
  consumption; duplicating it across two packages would mean keeping two
  copies in sync for no behavioral difference.
- The builder-sdk identity layer is already provider-agnostic by
  configuration: switching between Auth0.com (primary) and the pymthouse OIDC
  issuer (alternative) is `jwtIssuer` / `jwtAudience` / `claimMapping` env
  values, not a code fork. Hosted vs. on-prem follows the same philosophy —
  it's where the Kafka collector, signer DMZ, and OpenMeter ingest point live,
  which is infrastructure/config, not application code.
- A single `.env` (mirroring `auth0-livepeer`'s `.env.livepeer` bootstrap
  output) drives the webhook, the collector, and the SDK together. Two
  packages would mean two things to keep in sync per deployment.

What **does** differ between modes lives outside the npm package, as
infrastructure artifacts selected per deployment:

|                                | Hosted                         | On-prem / self-hosted                                     |
| ------------------------------ | ------------------------------ | --------------------------------------------------------- |
| Kafka + collector + signer DMZ | Managed (e.g. Railway compose) | Single-VM Docker Compose (Ansible-provisioned)            |
| Usage storage                  | OpenMeter (Konnect)            | OpenMeter collector container + platform-owned SQL schema |
| App package                    | `clearinghouse` (this repo)    | `clearinghouse` (this repo)                               |

If a real behavioral fork ever emerges (not just config), split it into a
second package at that point — don't pre-build it speculatively.

## Architecture (planned)

```
go-livepeer  ──POST /authorize──▶  clearinghouse webhook ──▶ builder-sdk
  (signer)                          (OIDC / API-key /         (UsageIdentity,
                                      custom verifier)          M2M tokens)

go-livepeer  ──create_signed_ticket──▶ Kafka ──▶ Benthos collector ──▶ OpenMeter
  (signer DMZ)                        (async, decoupled from signing path)
```

- **Webhook** (`/authorize`): `createRemoteSignerAuthorizeHandler` from
  `@pymthouse/builder-sdk/signer/webhook`, fail-closed on missing
  `WEBHOOK_SECRET`.
- **Identity**: OIDC by default (Auth0.com primary, pymthouse OIDC issuer as a
  drop-in alternative — config only), with API-key and custom verifier
  examples.
- **Metering**: asynchronous, via go-livepeer's `create_signed_ticket` Kafka
  topic → Benthos → OpenMeter (Konnect). Decoupled from the signing path.
- **Admin**: a thin wrapper over builder-sdk's billing/usage/allowance helpers
  (`getUsageBalance`, `getUserAllowances`, `grantUserAllowance`, `getUsage`,
  ...) for entitlement management and reporting.

## Requirements

- Node.js >= 20 (CI runs 22 and 24; `.nvmrc` pins 22)
- [pnpm](https://pnpm.io/) 10.x

## Getting started

```bash
pnpm install
pnpm dev        # run the webhook server locally
pnpm build      # compile src/ -> dist/
pnpm start      # run the compiled server
```

| Script                              | Purpose                              |
| ----------------------------------- | ------------------------------------ |
| `pnpm dev`                          | Run `src/index.ts` with live reload  |
| `pnpm build`                        | Type-check + emit `dist/` via `tsc`  |
| `pnpm start`                        | Run the compiled server from `dist/` |
| `pnpm typecheck`                    | `tsc --noEmit`                       |
| `pnpm lint`                         | ESLint, zero warnings                |
| `pnpm test`                         | Vitest                               |
| `pnpm format` / `pnpm format:check` | Prettier                             |

Currently `pnpm dev` / `pnpm start` only serve `GET /healthz` — enough to prove
the package builds, runs, and resolves `@pymthouse/builder-sdk`. The
`/authorize` route lands in a follow-up issue.

## Relationship to other repos

- [`pymthouse/builder-sdk`](https://github.com/pymthouse/builder-sdk) (v0.4.1)
  — the SDK this repo consumes. See its README for the subpath export table
  and the "Remote signer identity webhook" section.
- [`pymthouse/pymthouse` PR
  #133](https://github.com/pymthouse/pymthouse/pull/133) — the hosted
  reference implementation (`src/app/webhooks/remote-signer/route.ts`, Kafka +
  Benthos + signer DMZ on Railway). Used as the implementation reference for
  the webhook and collector here.
- [`pymthouse/auth0-livepeer` PR
  #1](https://github.com/pymthouse/auth0-livepeer/pull/1) — Auth0.com
  provisioning scripts (`scripts/bootstrap.ts`, `config/meters.json`) that
  produce `.env.livepeer`, plus the single-VM on-prem runtime stack (Ansible +
  Docker Compose). Used as the primary identity-provisioning and on-prem
  packaging reference.

## License

[MIT](LICENSE)
