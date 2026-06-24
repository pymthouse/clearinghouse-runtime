import { createServer } from "node:http";
import {
  createApiKeyEndUserVerifier,
  routeRemoteSignerWebhookRequest,
} from "@pymthouse/builder-sdk/signer/webhook";
import { loadApiKeyStore } from "./keys.mjs";

const port = Number(process.env.PORT || 8090);

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

const keyStore = loadApiKeyStore(process.env);

const config = {
  webhookSecret: required("WEBHOOK_SECRET"),
  endUserAuth: createApiKeyEndUserVerifier({
    issuer: required("IDENTITY_ISSUER"),
    apiKeyPrefix: process.env.API_KEY_PREFIX?.trim() || "sk_",
    defaultClientId: process.env.DEMO_CLIENT_ID?.trim() || "demo-client",
    defaultUsageSubjectType: process.env.USAGE_SUBJECT_TYPE?.trim() || "api_key_user",
    resolveApiKey: async (apiKey) => {
      const entry = keyStore.get(apiKey);
      if (!entry) {
        return null;
      }
      return {
        userId: entry.userId,
        clientId: entry.clientId,
        usageSubjectType: entry.usageSubjectType,
      };
    },
  }),
};

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks)));
    req.on("error", reject);
  });
}

async function handleRequest(req, res) {
  if (req.method === "GET" && req.url === "/health") {
    res.writeHead(200, { "Content-Type": "text/plain" });
    res.end("ok");
    return;
  }

  const host = req.headers.host || `localhost:${port}`;
  const body =
    req.method === "GET" || req.method === "HEAD" ? undefined : await readBody(req);
  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (value === undefined) {
      continue;
    }
    headers.set(key, Array.isArray(value) ? value.join(", ") : value);
  }

  const request = new Request(`http://${host}${req.url}`, {
    method: req.method,
    headers,
    body: body?.length ? body : undefined,
  });

  const response = await routeRemoteSignerWebhookRequest(request, config);
  if (!response) {
    res.writeHead(404);
    res.end();
    return;
  }

  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(await response.text());
}

createServer((req, res) => {
  handleRequest(req, res).catch((err) => {
    console.error("identity-webhook error:", err);
    if (!res.headersSent) {
      res.writeHead(500, { "Content-Type": "text/plain" });
    }
    res.end("internal error");
  });
}).listen(port, "0.0.0.0", () => {
  console.log(`identity-webhook (api-key) listening on :${port}`);
});
