# Clearinghouse Kafka bus (Redpanda dev-container mode).
# KAFKA_ADVERTISED_ADDR: kafka:9092 for local docker-compose (default);
# set kafka.railway.internal:9092 via railway-apply-stack-env.sh for Railway.
FROM docker.redpanda.com/redpandadata/redpanda:v24.2.4

ENV CLUSTER_ID=6ac74f21-e6d3-453b-b5d6-de4fb811e8bb
ENV KAFKA_ADVERTISED_ADDR=kafka:9092

CMD redpanda start \
  --kafka-addr internal://0.0.0.0:9092 \
  --advertise-kafka-addr internal://${KAFKA_ADVERTISED_ADDR} \
  --mode dev-container \
  --smp 1 \
  --memory 512M \
  --overprovisioned
