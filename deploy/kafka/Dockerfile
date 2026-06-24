# Clearinghouse Kafka bus (Redpanda dev-container mode).
# KAFKA_ADVERTISED_ADDR defaults to kafka:9092 for local docker-compose.
FROM docker.redpanda.com/redpandadata/redpanda:v24.2.4

ENV KAFKA_ADVERTISED_ADDR=kafka:9092

USER root
COPY deploy/kafka/entrypoint.sh /clearinghouse-entrypoint.sh
RUN chmod +x /clearinghouse-entrypoint.sh
USER redpanda

ENTRYPOINT ["/clearinghouse-entrypoint.sh"]
