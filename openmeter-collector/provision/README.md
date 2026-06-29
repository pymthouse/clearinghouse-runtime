# OpenMeter / Konnect metering bootstrap

Idempotent provisioning of the clearinghouse metering catalog (meters, features,
plan check) and per-tenant customers, driven by [`kongctl`](https://developer.konghq.com/kongctl/)
against the Konnect Metering & Billing (OpenMeter) API.

`kongctl` has no native meter resource yet ([Kong/kongctl#1334](https://github.com/Kong/kongctl/issues/1334)),
so these scripts use `kongctl api` — its authenticated passthrough to the Konnect REST API —
to drive `/v3/openmeter/*`. The catalog is data, defined in [`catalog.json`](catalog.json);
the scripts are thin and idempotent.

## Prerequisites

- `kongctl` and `jq` on `PATH` (the PowerShell script needs only `kongctl`).
- A Konnect Personal Access Token (`kpat_…`) with Metering & Billing access.

## Configuration (env)

| Variable | Purpose | Default |
| --- | --- | --- |
| `KONGCTL_DEFAULT_KONNECT_PAT` | Konnect PAT (preferred). Falls back to `OPENMETER_API_KEY`. | — (required) |
| `OPENMETER_API_KEY` | Same PAT, reused from `openmeter-collector/.env`. | — |
| `OPENMETER_URL` | Metering API base. | `https://us.api.konghq.com/v3/openmeter` |

The values in [`openmeter-collector/.env`](../.env.example) are sufficient:

```bash
set -a; source openmeter-collector/.env; set +a
```

## Usage

```bash
# Catalog only — ensure meters + features, verify the plan exists.
./bootstrap.sh catalog

# Provision one tenant customer (key = client_id:external_user_id).
./bootstrap.sh customer demo-client demo-user "Demo User"

# Catalog + customer in one run; --subscribe also ensures a plan subscription.
./bootstrap.sh all demo-client demo-user "Demo User" --subscribe
```

Windows (PowerShell):

```powershell
.\bootstrap.ps1 catalog
.\bootstrap.ps1 customer demo-client demo-user "Demo User"
.\bootstrap.ps1 all demo-client demo-user "Demo User" -Subscribe
```

Both scripts are safe to re-run: existing meters/features/customers are reported and
left untouched.

## What it provisions

From [`catalog.json`](catalog.json):

| Kind | Key | Notes |
| --- | --- | --- |
| Meter | `network_fee_usd_micros` | SUM of `$.network_fee_usd_micros` |
| Meter | `billable_usd_micros` | SUM of `$.billable_usd_micros` (interim = network fee until phase-2 markup) |
| Meter | `signed_ticket_count` | COUNT |
| Feature | `network_spend` | → `network_fee_usd_micros` |
| Feature | `billable_spend` | → `billable_usd_micros` |
| Plan | `clearinghouse_default_ppu` | **verified, not created** — author plans in Konnect (rate cards/pricing) |

## Identity contract (important)

The CloudEvent **`subject` is the compound `client_id:usage_subject`** (e.g.
`demo-client:demo-user`), which is also the customer key and its single `subject_key`.
OpenMeter attributes usage by exact subject match, and **forbids changing a customer's
`subject_keys` once it has an active subscription** — so the subject must be compound and
correct from creation. Break usage down per-tenant/user with the `client_id` / `usage_subject`
meter dimensions, not by changing the subject. The scripts therefore never mutate
`subject_keys` on existing customers; they warn if an existing customer is missing the
expected compound key.

## Limitations

- Customer lookup lists customers and exact-matches the key locally (the API `key`
  filter is a partial match). For very large customer bases, add pagination.
- Plans and subscriptions: the script verifies the plan and best-effort-creates a
  subscription with `--subscribe`; full plan/rate-card authoring is out of scope (use
  the Konnect UI/API).
