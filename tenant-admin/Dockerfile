FROM node:22-bookworm-slim

WORKDIR /app
COPY tenant-admin/package.json ./
COPY tenant-admin/registry.mjs tenant-admin/server.mjs tenant-admin/cli.mjs ./
RUN mkdir -p /app/data

ENV PORT=8093
ENV TENANT_ADMIN_DATA_DIR=/app/data

EXPOSE 8093
CMD ["node", "server.mjs"]
