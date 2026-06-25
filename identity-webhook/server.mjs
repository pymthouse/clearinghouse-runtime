import { createServer } from "node:http";
import {
  createApiKeyEndUserVerifier,
  createFirstMatchEndUserVerifier,
  createOidcEndUserVerifier,
  routeRemoteSignerWebhookRequest,
} from "@pymthouse/builder-sdk/signer/webhook";
import { buildAuthId } from "./billing-identity.mjs";
import { loadApiKeyStore } from "./keys.mjs";
import { createTenantRegistryIndex } from "./tenant-registry.mjs";

const port = Number(process.env.PORT || 8090);

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function defaultSignerWebhookJwtAudience(jwtIssuer) {
  return jwtIssuer.trim().replace(/\/+$/, "");
}

const tenantRegistry = createTenantRegistryIndex(process.env);
const keyStore = loadApiKeyStore(process.env);

const apiKeyVerifier = createApiKeyEndUserVerifier({
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
      tenantId: entry.tenantId,
      clientId: entry.clientId,
      usageSubjectType: entry.usageSubjectType,
    };
  },
});

function createEndUserAuth() {
  const jwtIssuer = process.env.JWT_ISSUER?.trim();
  if (!jwtIssuer) {
    return apiKeyVerifier;
  }

  const jwtAudience =
    process.env.JWT_AUDIENCE?.trim() || defaultSignerWebhookJwtAudience(jwtIssuer);

  const oidcVerifier = createOidcEndUserVerifier({
    webhookSecret: required("WEBHOOK_SECRET"),
    jwtIssuer,
    jwtAudience,
    claimMapping: {
      claimClientId: process.env.CLAIM_CLIENT_ID?.trim() || "azp",
      claimUsageSubject: process.env.CLAIM_USAGE_SUBJECT?.trim() || "sub",
      usageSubjectType: process.env.USAGE_SUBJECT_TYPE?.trim() || "auth0_user_id",
    },
    allowInsecureHttp: process.env.ALLOW_INSECURE_HTTP?.trim() === "1",
  });

  return createFirstMatchEndUserVerifier([apiKeyVerifier, oidcVerifier]);
}

const config = {
  webhookSecret: required("WEBHOOK_SECRET"),
  endUserAuth: createEndUserAuth(),
};

function enrichAuthorizeResponse(responseText, rawBody) {
  try {
    const payload = JSON.parse(responseText);
    if (payload?.status !== 200 || !payload?.identity) {
      return responseText;
    }

    const identity = payload.identity;
    const apiKey = extractEndUserApiKey(rawBody);
    const keyEntry = apiKey ? keyStore.get(apiKey) : undefined;

    let tenantId = keyEntry?.tenantId?.trim() || null;
    let billingClientId = keyEntry?.clientId?.trim() || null;
    const jwtClientId = String(identity.client_id ?? "").trim();
    const jwtAuth0ClientId =
      process.env.CLAIM_CLIENT_ID?.trim() === "azp" ? jwtClientId : "";

    if (!tenantId) {
      tenantId = tenantRegistry.resolveTenantId(jwtClientId, jwtAuth0ClientId);
    }
    if (!billingClientId) {
      billingClientId =
        tenantRegistry.billingClientId(jwtClientId, jwtAuth0ClientId) || jwtClientId;
    }

    if (tenantId && billingClientId && identity.usage_subject) {
      payload.auth_id = buildAuthId(tenantId, billingClientId, identity.usage_subject);
      return JSON.stringify(payload);
    }

    return responseText;
  } catch {
    return responseText;
  }
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks)));
    req.on("error", reject);
  });
}

function extractEndUserApiKey(rawBody) {
  if (!rawBody?.length) {
    return "";
  }
  try {
    const payload = JSON.parse(rawBody.toString("utf8"));
    const headerValues = payload?.headers?.Authorization ?? payload?.headers?.authorization;
    const authorization = Array.isArray(headerValues) ? headerValues[0] : headerValues;
    if (typeof authorization !== "string") {
      return "";
    }
    const match = authorization.match(/^Bearer\s+(.+)$/i);
    return match?.[1]?.trim() ?? "";
  } catch {
    return "";
  }
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

  let responseText = await response.text();
  if (req.method === "POST" && req.url === "/authorize" && response.status === 200) {
    responseText = enrichAuthorizeResponse(responseText, body);
  }

  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(responseText);
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
  const mode = process.env.JWT_ISSUER?.trim() ? "api-key + auth0 oidc" : "api-key";
  console.log(`identity-webhook (${mode}) listening on :${port}`);
});
