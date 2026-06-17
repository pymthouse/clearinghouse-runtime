import { createServer } from "node:http";
import { createAuth0ManagementClient, createAuth0UserProvisioner } from "@pymthouse/builder-sdk/auth0/management";
import {
  createOpenMeterBillingProvisioner,
  createOpenMeterClient,
} from "@pymthouse/builder-sdk/billing/openmeter";
import {
  createAuth0BillingWebhookConfig,
  routeRemoteSignerWebhookRequest,
} from "@pymthouse/builder-sdk/signer/webhook";

const port = Number(process.env.PORT || 8090);

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`${name} is required`);
  }
  return value;
}

function optional(name) {
  const value = process.env[name]?.trim();
  return value || undefined;
}

function buildConfig() {
  const openMeterClient = createOpenMeterClient({
    baseUrl: required("OPENMETER_URL"),
    apiKey: required("OPENMETER_API_KEY"),
  });

  const defaultPlanKey = required("OPENMETER_DEFAULT_PLAN_KEY");
  const billingClientId = required("AUTH0_PUBLIC_CLIENT_ID");

  const billingProvisioner = createOpenMeterBillingProvisioner({
    client: openMeterClient,
    resolvePlanKey: () => defaultPlanKey,
  });

  const mgmtClientId = optional("AUTH0_MGMT_CLIENT_ID");
  const mgmtClientSecret = optional("AUTH0_MGMT_CLIENT_SECRET");
  const auth0Domain = optional("AUTH0_DOMAIN");

  const userProvisioner =
    auth0Domain && mgmtClientId && mgmtClientSecret
      ? createAuth0UserProvisioner({
          management: createAuth0ManagementClient({
            domain: auth0Domain,
            clientId: mgmtClientId,
            clientSecret: mgmtClientSecret,
          }),
          defaultConnection:
            process.env.AUTH0_DEFAULT_CONNECTION?.trim() || "Username-Password-Authentication",
        })
      : undefined;

  return createAuth0BillingWebhookConfig({
    webhookSecret: required("WEBHOOK_SECRET"),
    jwtIssuer: required("JWT_ISSUER"),
    jwtAudience: required("JWT_AUDIENCE"),
    claimMapping: {
      claimClientId: process.env.CLAIM_CLIENT_ID?.trim() || "azp",
      usageSubjectType: process.env.USAGE_SUBJECT_TYPE?.trim() || "auth0_user_id",
    },
    allowInsecureHttp: true,
    billingProvisioner,
    userProvisioner,
    defaultBillingClientId: billingClientId,
    strictBillingProvision: process.env.STRICT_BILLING_PROVISION === "1",
    onBillingProvisionError: (err, identity) => {
      console.warn(
        "lazy billing provision failed:",
        err instanceof Error ? err.message : err,
        identity,
      );
    },
  });
}

const config = buildConfig();

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
  console.log(`identity-webhook listening on :${port}`);
});
