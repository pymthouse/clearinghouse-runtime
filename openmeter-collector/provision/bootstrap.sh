#!/usr/bin/env bash
#
# Bootstrap the OpenMeter/Konnect metering catalog for the clearinghouse collector.
#
# Idempotent: creates only what is missing. Meters are immutable in OpenMeter, so
# this script never updates or deletes them — it only adds missing meters/features
# and (for customers) appends missing usage-attribution subject keys.
#
# Requires: kongctl (https://developer.konghq.com/kongctl/) and jq.
# Auth:     KONGCTL_DEFAULT_KONNECT_PAT (preferred) or OPENMETER_API_KEY — a Konnect PAT (kpat_…).
# Endpoint: OPENMETER_URL (default https://us.api.konghq.com/v3/openmeter).
#
# Usage:
#   ./bootstrap.sh catalog
#       Ensure meters + features exist and the configured plan is present.
#
#   ./bootstrap.sh customer <client_id> <external_user_id> [display_name] [--subscribe]
#       Ensure an OpenMeter customer keyed <client_id>:<external_user_id> exists with
#       subject_keys = [<external_user_id> (bare, matches the CloudEvent subject),
#                       <client_id>:<external_user_id> (compound, tenant-scoped)].
#       --subscribe also ensures a subscription on the catalog plan (best-effort).
#
#   ./bootstrap.sh all <client_id> <external_user_id> [display_name] [--subscribe]
#       catalog + customer in one run.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CATALOG="${CATALOG:-$SCRIPT_DIR/catalog.json}"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*" >&2; }
warn() { printf 'warning: %s\n' "$*" >&2; }

command -v kongctl >/dev/null 2>&1 || die "kongctl not found (https://developer.konghq.com/kongctl/)"
command -v jq >/dev/null 2>&1 || die "jq not found"
[ -f "$CATALOG" ] || die "catalog not found: $CATALOG"

# --- auth + endpoint -------------------------------------------------------
PAT="${KONGCTL_DEFAULT_KONNECT_PAT:-${OPENMETER_API_KEY:-}}"
[ -n "$PAT" ] || die "set KONGCTL_DEFAULT_KONNECT_PAT or OPENMETER_API_KEY (a Konnect PAT)"
export KONGCTL_DEFAULT_KONNECT_PAT="$PAT"

OPENMETER_URL="${OPENMETER_URL:-https://us.api.konghq.com/v3/openmeter}"
OPENMETER_URL="${OPENMETER_URL%/}"
BASE="$(printf '%s' "$OPENMETER_URL" | sed -E 's#(https?://[^/]+).*#\1#')"
PREFIX="$(printf '%s' "$OPENMETER_URL" | sed -E 's#https?://[^/]+##')"
[ -n "$PREFIX" ] || PREFIX="/v3/openmeter"

# --- kongctl api helpers (return response body as JSON on stdout) ----------
kapi_get()  { kongctl api get    "$PREFIX$1" --base-url "$BASE" -o json; }
kapi_post() { kongctl api post   "$PREFIX$1" --base-url "$BASE" -o json -f -; }
kapi_put()  { kongctl api put    "$PREFIX$1" --base-url "$BASE" -o json -f -; }

# Unwrap list responses: collections come back as {"data":[...]}, items bare.
list_items() { jq -c '(.data // .)'; }

# --- catalog ---------------------------------------------------------------
ensure_meters() {
  local existing
  existing="$(kapi_get /meters | jq -r '(.data // .)[].key')"
  while IFS= read -r m; do
    [ -n "$m" ] || continue
    local key; key="$(jq -r '.key' <<<"$m")"
    if printf '%s\n' "$existing" | grep -qxF "$key"; then
      info "meter   $key — exists"
      continue
    fi
    local body
    body="$(jq '{name, key, description, event_type, aggregation}
              + (if .value_property then {value_property} else {} end)
              + (if .dimensions     then {dimensions}     else {} end)' <<<"$m")"
    printf '%s' "$body" | kapi_post /meters >/dev/null
    info "meter   $key — created"
  done < <(jq -c '.meters[]' "$CATALOG")
}

ensure_features() {
  local existing
  existing="$(kapi_get /features | jq -r '(.data // .)[].key')"
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    local key; key="$(jq -r '.key' <<<"$f")"
    if printf '%s\n' "$existing" | grep -qxF "$key"; then
      info "feature $key — exists"
      continue
    fi
    printf '%s' "$(jq '{key, name, meter_slug}' <<<"$f")" | kapi_post /features >/dev/null
    info "feature $key — created"
  done < <(jq -c '.features[]' "$CATALOG")
}

verify_plan() {
  local plan_key; plan_key="$(jq -r '.plan_key // empty' "$CATALOG")"
  [ -n "$plan_key" ] || { info "plan    — none configured"; return 0; }
  if kapi_get /plans | jq -e --arg k "$plan_key" '(.data // .)[]? | select(.key == $k)' >/dev/null; then
    info "plan    $plan_key — present"
  else
    warn "plan    $plan_key — NOT found; create it in Konnect before subscribing customers"
  fi
}

cmd_catalog() {
  info "== catalog ($BASE$PREFIX) =="
  ensure_meters
  ensure_features
  verify_plan
}

# --- customer --------------------------------------------------------------
# Find a customer by exact key. NOTE: the list filter is a partial match, so we
# fetch and exact-match locally. For very large customer bases add pagination.
find_customer() {
  kapi_get "/customers" | jq -c --arg k "$1" '(.data // .)[] | select(.key == $k)' | head -n 1
}

ensure_customer() {
  local client_id="$1" external_user_id="$2" display="$3" subscribe="$4"
  [ -n "$client_id" ] && [ -n "$external_user_id" ] || die "customer requires <client_id> <external_user_id>"
  # The CloudEvent subject is the compound client_id:external_user_id (globally
  # unique, tenant-scoped). It is also the customer key and its only subject_key.
  # OpenMeter forbids changing subject_keys once a customer has an active
  # subscription, so we set it correctly at creation and never mutate it.
  local compound="$client_id:$external_user_id"
  [ -n "$display" ] || display="$compound"

  local cust; cust="$(find_customer "$compound")"
  local id
  if [ -z "$cust" ]; then
    local body
    body="$(jq -n --arg key "$compound" --arg name "$display" \
      '{key:$key, name:$name, usage_attribution:{subject_keys:[$key]}}')"
    id="$(printf '%s' "$body" | kapi_post /customers | jq -r '.id')"
    info "customer $compound — created (subject: $compound)"
  else
    id="$(jq -r '.id' <<<"$cust")"
    if jq -e --arg c "$compound" '(.usage_attribution.subject_keys // []) | index($c)' <<<"$cust" >/dev/null; then
      info "customer $compound — up to date"
    else
      warn "customer $compound exists but its subject_keys do not include '$compound'"
      warn "  (OpenMeter blocks subject_key changes on subscribed customers — reconcile manually)"
    fi
  fi

  [ "$subscribe" = "1" ] && ensure_subscription "$id" "$compound" || true
}

# Best-effort subscription on the catalog plan. Skips if the customer already has one.
ensure_subscription() {
  local customer_id="$1" label="$2"
  local plan_key; plan_key="$(jq -r '.plan_key // empty' "$CATALOG")"
  [ -n "$plan_key" ] || { warn "no plan_key in catalog; skipping subscription"; return 0; }

  local existing
  existing="$(kapi_get "/subscriptions?customer_id=$customer_id" 2>/dev/null \
    | jq -r --arg c "$customer_id" '(.data // .)[]? | select(.customer_id == $c) | .id' 2>/dev/null || true)"
  if [ -n "$existing" ]; then
    info "sub      $label — exists"
    return 0
  fi
  local now body
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  body="$(jq -n --arg c "$customer_id" --arg p "$plan_key" --arg t "$now" \
    '{customer_id:$c, plan_key:$p, active_from:$t}')"
  if printf '%s' "$body" | kapi_post /subscriptions >/dev/null 2>&1; then
    info "sub      $label — created on $plan_key"
  else
    warn "sub      $label — could not create subscription on $plan_key (create manually if needed)"
  fi
}

# --- arg parsing -----------------------------------------------------------
SUBSCRIBE=0
ARGS=()
for a in "$@"; do
  case "$a" in
    --subscribe) SUBSCRIBE=1 ;;
    *) ARGS+=("$a") ;;
  esac
done
if [ "${#ARGS[@]}" -eq 0 ]; then set -- catalog; else set -- "${ARGS[@]}"; fi

cmd="${1:-catalog}"; shift || true
case "$cmd" in
  catalog)
    cmd_catalog
    ;;
  customer)
    ensure_customer "${1:-}" "${2:-}" "${3:-}" "$SUBSCRIBE"
    ;;
  all)
    cmd_catalog
    ensure_customer "${1:-}" "${2:-}" "${3:-}" "$SUBSCRIBE"
    ;;
  *)
    die "unknown command '$cmd' (expected: catalog | customer | all)"
    ;;
esac
info "done."
