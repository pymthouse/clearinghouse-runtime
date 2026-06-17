# OpenMeter Benthos collector: Kafka create_signed_ticket events -> Konnect/OpenMeter ingest.
FROM ghcr.io/openmeterio/benthos-collector:latest

COPY deploy/openmeter-collector/collector.yaml /config.yaml

CMD ["-c", "/config.yaml"]
