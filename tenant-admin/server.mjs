import { createServer } from "node:http";
import { createTenantRegistry } from "./registry.mjs";

const port = Number(process.env.PORT || 8093);
const adminSecret = process.env.ADMIN_SECRET?.trim() || "";
const registry = createTenantRegistry({
  dataDir: process.env.TENANT_ADMIN_DATA_DIR?.trim(),
});

function unauthorized(res) {
  res.writeHead(401, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ error: "unauthorized" }));
}

function isAuthorized(req) {
  const header = req.headers.authorization ?? "";
  const token = header.startsWith("Bearer ") ? header.slice(7).trim() : "";
  return adminSecret !== "" && token === adminSecret;
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks)));
    req.on("error", reject);
  });
}

async function readJson(req) {
  const raw = await readBody(req);
  if (!raw.length) {
    return {};
  }
  return JSON.parse(raw.toString("utf8"));
}

function writeJson(res, status, body) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

async function handleRequest(req, res) {
  const url = new URL(req.url ?? "/", `http://${req.headers.host ?? "localhost"}`);

  if (req.method === "GET" && url.pathname === "/health") {
    res.writeHead(200, { "Content-Type": "text/plain" });
    res.end("ok");
    return;
  }

  if (!url.pathname.startsWith("/admin/")) {
    res.writeHead(404);
    res.end();
    return;
  }

  if (!isAuthorized(req)) {
    unauthorized(res);
    return;
  }

  try {
    if (req.method === "GET" && url.pathname === "/admin/tenants") {
      const tenants = await registry.listTenants();
      const apps = await registry.listApps();
      writeJson(res, 200, { tenants, apps });
      return;
    }

    if (req.method === "POST" && url.pathname === "/admin/tenants") {
      const body = await readJson(req);
      const result = await registry.provisionTenant({
        tenantId: body.tenantId,
        tenantName: body.tenantName,
        clientId: body.clientId,
        auth0ClientId: body.auth0ClientId,
      });
      writeJson(res, 200, result);
      return;
    }

    if (req.method === "POST" && url.pathname === "/admin/apps/auth0") {
      const body = await readJson(req);
      const app = await registry.registerAuth0App({
        tenantId: body.tenantId,
        tenantName: body.tenantName,
        clientId: body.clientId,
        auth0ClientId: body.auth0ClientId,
      });
      writeJson(res, 200, { app });
      return;
    }

    const tenantMatch = url.pathname.match(/^\/admin\/tenants\/([^/]+)$/);
    if (req.method === "GET" && tenantMatch) {
      const tenantId = decodeURIComponent(tenantMatch[1]);
      const tenant = await registry.getTenant(tenantId);
      if (!tenant) {
        writeJson(res, 404, { error: "tenant not found" });
        return;
      }
      const apps = await registry.listApps(tenantId);
      writeJson(res, 200, { tenant, apps });
      return;
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : "request failed";
    writeJson(res, 400, { error: message });
    return;
  }

  res.writeHead(404);
  res.end();
}

if (!adminSecret) {
  console.error("ADMIN_SECRET is required");
  process.exit(1);
}

createServer((req, res) => {
  handleRequest(req, res).catch((err) => {
    console.error("tenant-admin error:", err);
    if (!res.headersSent) {
      res.writeHead(500, { "Content-Type": "text/plain" });
    }
    res.end("internal error");
  });
}).listen(port, "0.0.0.0", () => {
  console.log(`tenant-admin listening on :${port}`);
});
