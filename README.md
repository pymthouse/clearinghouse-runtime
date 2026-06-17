# clearinghouse

A single, cross-platform Go CLI that provisions **Auth0** and
**OpenMeter/Konnect** for the Livepeer remote-signer platform and emits
`.env.livepeer` + `sdk-config.json` for
[`@pymthouse/builder-sdk`](https://github.com/pymthouse/builder-sdk).

## Quick start

```bash
make build
cp .env.example .env    # fill in secrets — see file for Auth0 + Konnect vars
./clearinghouse-bootstrap

# Konnect only (no Auth0 creds in .env needed):
./clearinghouse-bootstrap --skip-auth0

# Auth0 only:
./clearinghouse-bootstrap --skip-openmeter
```

The CLI loads `.env` from the current directory automatically. See
`.env.example` for settings; run `./clearinghouse-bootstrap --help` for CLI flags.

## What it does

1. **Auth0** — creates a resource server (`livepeer-clearinghouse`, RS256, `sign:job`),
   a public client (native, device_code + refresh_token), an M2M client
   (client_credentials), and two client grants. Uses
   [`go-auth0/v2`](https://github.com/auth0/go-auth0).

2. **OpenMeter/Konnect** — idempotently ensures meters
   (`network_fee_usd_micros`, `billable_usd_micros`, `signed_ticket_count`),
   features (`network_spend`, `billable_spend`), and the default pay-per-use
   plan with a usage rate card. Uses the official
   [`Kong/sdk-konnect-go`](https://github.com/Kong/sdk-konnect-go) SDK.

3. **Output** — writes `.env.livepeer` (Auth0 + Konnect runtime vars) and
   `sdk-config.json` (structured config for Vercel platform deploy via
   builder-sdk). Signer URLs in `sdk-config.json` are placeholders to update
   after platform deploy.

## Configuration

Copy `.env.example` to `.env` and fill in your secrets. The CLI loads `.env`
automatically (override with `--env-file`).

If a required value is missing for the selected mode, the CLI exits with an
error before calling any APIs. See `.env.example` for variable names and
comments.

Use `--prune` to destructively remove Konnect catalog objects (meters,
features, plans) that are not defined in `config/meters.json` and
`config/pricing.json`, or whose meter dimensions no longer match config.
Prune runs before ensure/create. **This can delete production billing
catalog data** — use only when you intend to reconcile the tenant to config.

## Config files

Meter and pricing definitions live in `config/meters.json` and
`config/pricing.json`. These control which meters, features, and plans are
bootstrapped.

## Cross-compilation

```bash
make cross   # builds linux/darwin/windows on amd64/arm64 into dist/
```

Tagged releases (`v*`) are built and published via GitHub Actions
(`.github/workflows/release.yml`).

## Follow-ups (out of scope for this PR)

- Per-customer provisioning (customers + subscriptions)
- Self-hosted OpenMeter adapter
- Benthos collector / Docker Compose stack
- Railway deploy scripts
