# shellcheck shell=bash
# Shared Railway CLI auth helpers for clearinghouse deploy scripts.
#
# Token types (Railway docs):
#   RAILWAY_API_TOKEN — Account → Tokens (workspace scope). Use for CI + link/redeploy/up.
#   RAILWAY_TOKEN     — Project → Settings → Tokens (one env). Use for variable set / up only.
#
# Either token works when commands pass --project and --environment (no railway link needed).
# RAILWAY_PROJECT_ID must be set — no hardcoded default (create the project first).

railway_default_project_id() {
  if [[ -z "${RAILWAY_PROJECT_ID:-}" ]]; then
    echo "RAILWAY_PROJECT_ID is required. Set it to your Railway project ID." >&2
    echo "  Find it at: Railway dashboard → your project → Settings → General" >&2
    echo "  Set as a GitHub secret: gh secret set RAILWAY_PROJECT_ID -R livepeer/clearinghouse" >&2
    exit 1
  fi
  echo "$RAILWAY_PROJECT_ID"
}

railway_export_auth() {
  # The Railway CLI treats RAILWAY_TOKEN as a project token whenever it is PRESENT —
  # even as an empty string (e.g. in CI when the secret is unset). An empty project
  # token => Unauthorized even when RAILWAY_API_TOKEN is valid. Always unset the
  # token we're NOT using before invoking the CLI.
  if [[ -n "${RAILWAY_API_TOKEN:-}" ]]; then
    unset RAILWAY_TOKEN
    export RAILWAY_API_TOKEN
    return 0
  fi
  if [[ -n "${RAILWAY_TOKEN:-}" ]]; then
    unset RAILWAY_API_TOKEN
    export RAILWAY_TOKEN
    return 0
  fi
  unset RAILWAY_API_TOKEN RAILWAY_TOKEN
  if railway whoami >/dev/null 2>&1; then
    return 0
  fi
  echo "Railway auth required. Use ONE of:" >&2
  echo "  Account token (recommended for CI): Railway → Account → Tokens → create →" >&2
  echo "    export RAILWAY_API_TOKEN=<token>" >&2
  echo "    gh secret set RAILWAY_API_TOKEN -R livepeer/clearinghouse" >&2
  echo "  Project token: clearinghouse project → Settings → Tokens →" >&2
  echo "    export RAILWAY_TOKEN=<token>" >&2
  echo "  Local CLI login: unset RAILWAY_API_TOKEN RAILWAY_TOKEN && railway login" >&2
  return 1
}

railway_pe_flags() {
  echo "-p $(railway_default_project_id) -e $1"
}

railway_retryable_failure() {
  local err_file="$1"
  grep -qiE \
    'timed out|timeout|Failed to fetch|error sending request|connection reset|connection refused|temporarily unavailable|\b502\b|\b503\b|\b429\b' \
    "$err_file"
}

# Run a Railway CLI command with exponential backoff on transient failures.
# Usage: railway_retry railway variable set KEY=val ...
railway_retry() {
  local max_attempts="${RAILWAY_CLI_MAX_ATTEMPTS:-5}"
  local delay="${RAILWAY_CLI_RETRY_DELAY_SEC:-5}"
  local attempt=1
  local rc err_file

  err_file="$(mktemp)"

  while true; do
    : >"$err_file"
    if "$@" 2>"$err_file"; then
      rm -f "$err_file"
      return 0
    fi
    rc=$?
    cat "$err_file" >&2
    if [[ $attempt -ge $max_attempts ]] || ! railway_retryable_failure "$err_file"; then
      rm -f "$err_file"
      return "$rc"
    fi
    echo "Railway CLI attempt $attempt/$max_attempts failed (transient); retrying in ${delay}s..." >&2
    sleep "$delay"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
    if [[ $delay -gt 60 ]]; then
      delay=60
    fi
  done
}
