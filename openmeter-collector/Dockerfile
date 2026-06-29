# OpenMeter Benthos collector: Kafka create_signed_ticket events -> Konnect/OpenMeter ingest.
# Ingest-only: customers are ensured proactively by tenant-admin before service, not here.
FROM ghcr.io/openmeterio/benthos-collector:main-6b60ab6-1782310960 AS benthos

FROM alpine:3.21

RUN apk add --no-cache wget ca-certificates

WORKDIR /app

COPY openmeter-collector/entrypoint.sh /entrypoint.sh
COPY openmeter-collector/collector.yaml /config.yaml
COPY --from=benthos /usr/local/bin/benthos /usr/local/bin/benthos

RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
