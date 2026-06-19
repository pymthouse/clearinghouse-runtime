FROM golang:1.25.11-alpine AS build

WORKDIR /src
COPY deploy/billing/go.mod deploy/billing/go.sum ./
RUN go mod download
COPY deploy/billing/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /billing-gateway ./gateway

FROM alpine:3.21

RUN apk add --no-cache wget ca-certificates

COPY --from=build /billing-gateway /billing-gateway

ENV PORT=8092
ENV OPENMETER_DEFAULT_PLAN_KEY=clearinghouse_default_ppu
ENV OPENMETER_BALANCE_FEATURE_KEY=billable_spend

HEALTHCHECK --interval=10s --timeout=3s --retries=6 --start-period=10s \
  CMD wget -q -O- http://127.0.0.1:8092/health || exit 1

EXPOSE 8092

ENTRYPOINT ["/billing-gateway"]
