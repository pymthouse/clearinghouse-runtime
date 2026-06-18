# Clearinghouse Kafka bus (Redpanda dev-container mode).
# KAFKA_ADVERTISED_ADDR defaults to kafka:9092 for local docker-compose.
FROM docker.redpanda.com/redpandadata/redpanda:v24.2.4

ENV CLUSTER_ID=6ac74f21-e6d3-453b-b5d6-de4fb811e8bb
ENV KAFKA_ADVERTISED_ADDR=kafka:9092

USER root
COPY deploy/kafka/entrypoint.sh /clearinghouse-entrypoint.sh
RUN chmod +x /clearinghouse-entrypoint.sh
USER redpanda

ENTRYPOINT ["/clearinghouse-entrypoint.sh"]
