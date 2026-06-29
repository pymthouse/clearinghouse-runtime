#!/bin/sh
set -eu

if [ -f /service/.env ]; then
  set -a
  # shellcheck disable=SC1091
  . /service/.env
  set +a
fi

if [ -z "${OPENMETER_INGEST_URL:-}" ] || [ -z "${OPENMETER_API_KEY:-}" ]; then
  echo "entrypoint: OPENMETER_INGEST_URL and OPENMETER_API_KEY are required" >&2
  exit 1
fi

exec /usr/local/bin/benthos -c /config.yaml
