# OpenMeter Benthos collector: Kafka create_signed_ticket events -> Konnect/OpenMeter ingest.
FROM ghcr.io/openmeterio/benthos-collector:main-6b60ab6-1782310960

COPY openmeter-collector/collector.yaml /config.yaml
COPY openmeter-collector/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
