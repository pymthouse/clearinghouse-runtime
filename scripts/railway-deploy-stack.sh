#!/usr/bin/env bash
# Deploy the clearinghouse Railway stack (kafka + openmeter-collector + remote-signer).
# No Apache DMZ — remote-signer is a plain Dockerfile service like kafka and collector.
#
# Prerequisites:
#   - RAILWAY_API_TOKEN (account token) or RAILWAY_TOKEN (project token)
#   - RAILWAY_PROJECT_ID (Railway project with three services created)
#   - Env applied: bash scripts/railway-apply-stack-env.sh
#
# Usage:
#   RAILWAY_API_TOKEN=... RAILWAY_PROJECT_ID=... bash scripts/railway-deploy-stack.sh production
#   RAILWAY_API_TOKEN=... RAILWAY_PROJECT_ID=... bash scripts/railway-deploy-stack.sh preview
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

ENV="${1:-production}"
# shellcheck source=lib/railway-auth.sh
source "$ROOT/scripts/lib/railway-auth.sh"

if ! command -v railway >/dev/null 2>&1; then
  echo "Install Railway CLI: npm install -g @railway/cli" >&2
  exit 1
fi

railway_export_auth || exit 1

echo "=== Railway stack deploy: $ENV ==="

bash "$ROOT/scripts/railway-deploy-from-manifest.sh" kafka "$ENV" deploy/kafka
bash "$ROOT/scripts/railway-deploy-from-manifest.sh" openmeter-collector "$ENV" deploy/openmeter-collector
bash "$ROOT/scripts/railway-deploy-from-manifest.sh" remote-signer "$ENV" deploy/remote-signer

echo "=== Stack deploy triggered for $ENV ==="
echo "After deploy, confirm: signer on port 8081, collector consuming Kafka, events in OpenMeter."
