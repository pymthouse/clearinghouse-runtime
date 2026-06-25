import { mkdir, readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";

const DEFAULT_DATA_DIR = resolve(import.meta.dirname, "data");

function nowIso() {
  return new Date().toISOString();
}

function appsPath(dataDir) {
  return resolve(dataDir, "apps.json");
}

function tenantsPath(dataDir) {
  return resolve(dataDir, "tenants.json");
}

async function readJsonArray(path) {
  try {
    const raw = await readFile(path, "utf8");
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch (err) {
    if (err && typeof err === "object" && "code" in err && err.code === "ENOENT") {
      return [];
    }
    throw err;
  }
}

async function writeJsonArray(path, rows) {
  await mkdir(dirname(path), { recursive: true });
  await writeFile(path, `${JSON.stringify(rows, null, 2)}\n`, "utf8");
}

function normalizeAppRow(row) {
  const tenantId = String(row?.tenantId ?? "").trim();
  const clientId = String(row?.clientId ?? "").trim();
  const auth0ClientId = String(row?.auth0ClientId ?? "").trim();
  const tenantName = String(row?.tenantName ?? "").trim();
  if (!tenantId || !clientId) {
    throw new Error("tenantId and clientId are required");
  }
  return {
    tenantId,
    clientId,
    ...(auth0ClientId ? { auth0ClientId } : {}),
    ...(tenantName ? { tenantName } : {}),
    createdAt: row?.createdAt ?? nowIso(),
    updatedAt: nowIso(),
  };
}

function normalizeTenantRow(row) {
  const tenantId = String(row?.tenantId ?? "").trim();
  const tenantName = String(row?.tenantName ?? "").trim();
  if (!tenantId || !tenantName) {
    throw new Error("tenantId and tenantName are required");
  }
  return {
    tenantId,
    tenantName,
    createdAt: row?.createdAt ?? nowIso(),
    updatedAt: nowIso(),
  };
}

export function createTenantRegistry(options = {}) {
  const dataDir = resolve(options.dataDir ?? DEFAULT_DATA_DIR);

  return {
    dataDir,

    async listTenants() {
      return readJsonArray(tenantsPath(dataDir));
    },

    async listApps(tenantId) {
      const apps = await readJsonArray(appsPath(dataDir));
      const trimmed = String(tenantId ?? "").trim();
      if (!trimmed) {
        return apps;
      }
      return apps.filter((row) => row.tenantId === trimmed);
    },

    async getTenant(tenantId) {
      const tenants = await this.listTenants();
      return tenants.find((row) => row.tenantId === tenantId) ?? null;
    },

    async upsertTenant(input) {
      const next = normalizeTenantRow(input);
      const tenants = await this.listTenants();
      const index = tenants.findIndex((row) => row.tenantId === next.tenantId);
      if (index >= 0) {
        tenants[index] = { ...tenants[index], ...next, createdAt: tenants[index].createdAt };
      } else {
        tenants.push(next);
      }
      await writeJsonArray(tenantsPath(dataDir), tenants);
      return next;
    },

    async upsertApp(input) {
      const next = normalizeAppRow(input);
      const apps = await readJsonArray(appsPath(dataDir));
      const index = apps.findIndex(
        (row) => row.tenantId === next.tenantId && row.clientId === next.clientId,
      );
      if (index >= 0) {
        apps[index] = {
          ...apps[index],
          ...next,
          createdAt: apps[index].createdAt,
        };
      } else {
        apps.push(next);
      }
      await writeJsonArray(appsPath(dataDir), apps);
      return next;
    },

    async registerAuth0App({ tenantId, clientId, auth0ClientId, tenantName }) {
      const trimmedAuth0 = String(auth0ClientId ?? "").trim();
      if (!trimmedAuth0) {
        throw new Error("auth0ClientId is required");
      }
      if (tenantName) {
        await this.upsertTenant({ tenantId, tenantName });
      }
      return this.upsertApp({ tenantId, clientId, auth0ClientId: trimmedAuth0 });
    },

    async provisionTenant(input) {
      const tenant = await this.upsertTenant({
        tenantId: input.tenantId,
        tenantName: input.tenantName,
      });
      let app = null;
      if (input.clientId) {
        app = await this.upsertApp({
          tenantId: input.tenantId,
          clientId: input.clientId,
          auth0ClientId: input.auth0ClientId,
        });
      }
      return { tenant, app };
    },

    async lookupByClientId(clientId) {
      const trimmed = String(clientId ?? "").trim();
      if (!trimmed) {
        return null;
      }
      const apps = await readJsonArray(appsPath(dataDir));
      return apps.find((row) => row.clientId === trimmed) ?? null;
    },

    async lookupByAuth0ClientId(auth0ClientId) {
      const trimmed = String(auth0ClientId ?? "").trim();
      if (!trimmed) {
        return null;
      }
      const apps = await readJsonArray(appsPath(dataDir));
      return apps.find((row) => row.auth0ClientId === trimmed) ?? null;
    },

    async loadClientTenantMap() {
      const apps = await readJsonArray(appsPath(dataDir));
      const map = new Map();
      for (const row of apps) {
        if (row.clientId && row.tenantId) {
          map.set(row.clientId, row.tenantId);
        }
        if (row.auth0ClientId && row.tenantId) {
          map.set(row.auth0ClientId, row.tenantId);
        }
      }
      return map;
    },
  };
}
