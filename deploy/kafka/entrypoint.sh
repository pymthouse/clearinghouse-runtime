#!/bin/sh
set -eu

ADVERTISED="${KAFKA_ADVERTISED_ADDR:-kafka:9092}"

# NOTE: Deployment: See https://github.com/livepeer/clearinghouse/issues/43 for tracking.
# The --mode dev-container flag below launches Redpanda in "development container" mode:
# - No security/encryption (PLAINTEXT)
# - Not for production! This enables fast local startup, disables most persistence/durability, 
#   and relaxes networking for development and Docker Compose stacks.
# Update this script if production requirements change.

# Base image ENTRYPOINT is /entrypoint.sh -> exec /usr/bin/rpk "$@".
# Shell-form CMD becomes /bin/sh -c ..., which rpk rejects (-c flag).
# Exec-form CMD works; entrypoint allows runtime KAFKA_ADVERTISED_ADDR override.
exec /usr/bin/rpk redpanda start \
  --kafka-addr "internal://0.0.0.0:9092" \
  --advertise-kafka-addr "internal://${ADVERTISED}" \
  --mode dev-container \
  --smp 1 \
  --memory 512M \
  --overprovisioned
