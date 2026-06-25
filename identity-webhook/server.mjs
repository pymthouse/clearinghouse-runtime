import { createServer } from "node:http";
import {
  createApiKeyEndUserVerifier,
  createFirstMatchEndUserVerifier,
  createOidcEndUserVerifier,
  routeIdentityServiceRequest,
} from "@pymthouse/builder-sdk/signer/webhook";
import { buildAuthId } from "./billing-identity.mjs";
import { loadApiKeyStore } from "./keys.mjs";
import { createOpenMeterUsageReaders, isUsageQueryEnabled } from "./openmeter-read.mjs";

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
      claimClientId: process.env.CLAIM_CLIENT_ID?.trim() || "client_id",
      claimUsageSubject: process.env.CLAIM_USAGE_SUBJECT?.trim() || "sub",
      usageSubjectType: process.env.USAGE_SUBJECT_TYPE?.trim() || "external_user_id",
    },
    allowInsecureHttp: process.env.ALLOW_INSECURE_HTTP?.trim() === "1",
  });

  return createFirstMatchEndUserVerifier([apiKeyVerifier, oidcVerifier]);
}

const endUserAuth = createEndUserAuth();
const openMeterReaders = createOpenMeterUsageReaders(process.env, keyStore);

const config = {
  webhookSecret: required("WEBHOOK_SECRET"),
  endUserAuth,
  ...(openMeterReaders
    ? {
        endUserUsage: {
          readBalance: openMeterReaders.readBalance,
          readUsage: openMeterReaders.readUsage,
        },
      }
    : {}),
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

  const response = await routeIdentityServiceRequest(request, config);
  if (!response) {
    res.writeHead(404);
    res.end();
    return;
  }

  const responseText = await response.text();
  let outgoingBody = responseText;
  if (req.method === "POST" && req.url === "/authorize" && response.status === 200) {
    try {
      const payload = JSON.parse(responseText);
      if (payload?.status === 200 && payload?.identity) {
        const apiKey = extractEndUserApiKey(body);
        const entry = apiKey ? keyStore.get(apiKey) : undefined;
        const tenantId = entry?.tenantId?.trim();
        if (tenantId) {
          payload.auth_id = buildAuthId(
            tenantId,
            payload.identity.client_id,
            payload.identity.usage_subject,
          );
          outgoingBody = JSON.stringify(payload);
        }
      }
    } catch {
      // Keep original authorize response on parse errors.
    }
  }

  res.writeHead(response.status, Object.fromEntries(response.headers));
  res.end(outgoingBody);
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

createServer((req, res) => {
  handleRequest(req, res).catch((err) => {
    console.error("identity-webhook error:", err);
    if (!res.headersSent) {
      res.writeHead(500, { "Content-Type": "text/plain" });
    }
    res.end("internal error");
  });
}).listen(port, "0.0.0.0", () => {
  const usageMode = isUsageQueryEnabled(process.env) ? "openmeter usage reads" : "authorize only";
  console.log(`identity-webhook listening on :${port} (${usageMode})`);
});
