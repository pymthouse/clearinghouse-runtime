# Clearinghouse Kafka bus (Redpanda dev-container mode).
FROM docker.redpanda.com/redpandadata/redpanda:v24.2.4

USER root
COPY kafka/entrypoint.sh /clearinghouse-entrypoint.sh
RUN chmod +x /clearinghouse-entrypoint.sh
USER redpanda

ENTRYPOINT ["/clearinghouse-entrypoint.sh"]
