# OpenMeter Benthos collector: Kafka create_signed_ticket events -> Konnect/OpenMeter ingest.
FROM ghcr.io/openmeterio/benthos-collector:latest

USER root
RUN apk add --no-cache curl

COPY deploy/openmeter-collector/collector.yaml /config.yaml
COPY deploy/openmeter-collector/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["-c", "/config.yaml"]
