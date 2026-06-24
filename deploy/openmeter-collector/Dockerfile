# OpenMeter Benthos collector: Kafka create_signed_ticket events -> Konnect/OpenMeter ingest.
FROM ghcr.io/openmeterio/benthos-collector:main-6b60ab6-1782310960

COPY deploy/openmeter-collector/collector.yaml /config.yaml

CMD ["-c", "/config.yaml"]
