FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY tenant-admin/go.mod tenant-admin/go.sum ./
RUN go mod download

COPY tenant-admin/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/tenant-admin ./cmd/tenant-admin

FROM alpine:3.20
RUN adduser -D -H tenantadmin
WORKDIR /app
COPY --from=builder /out/tenant-admin /usr/local/bin/tenant-admin
USER tenantadmin
EXPOSE 8093
ENTRYPOINT ["/usr/local/bin/tenant-admin", "-mode", "server"]
