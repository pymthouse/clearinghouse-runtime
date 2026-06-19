FROM node:22-alpine

RUN apk add --no-cache curl

WORKDIR /app

COPY deploy/identity-webhook/package.json ./
RUN npm install --omit=dev

COPY deploy/identity-webhook/*.mjs ./

ENV PORT=8090

EXPOSE 8090

HEALTHCHECK --interval=10s --timeout=3s --retries=6 --start-period=5s \
  CMD curl -fsS http://localhost:8090/health || exit 1

CMD ["node", "server.mjs"]
