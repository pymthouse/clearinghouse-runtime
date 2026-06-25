import { readFileSync } from "node:fs";

function loadAppsFromRegistry(registryPath) {
  const raw = readFileSync(registryPath, "utf8");
  const parsed = JSON.parse(raw);
  if (!Array.isArray(parsed)) {
    throw new Error("tenant registry apps.json must be a JSON array");
  }
  return parsed;
}

export function createTenantRegistryIndex(env) {
  const registryPath = env.TENANT_REGISTRY_PATH?.trim();
  if (!registryPath) {
    return {
      lookupByClientId() {
        return null;
      },
      lookupByAuth0ClientId() {
        return null;
      },
      resolveTenantId() {
        return env.DEFAULT_TENANT_ID?.trim() || null;
      },
    };
  }

  const apps = loadAppsFromRegistry(registryPath);
  const byClientId = new Map();
  const byAuth0ClientId = new Map();

  for (const row of apps) {
    const tenantId = row?.tenantId?.trim?.();
    const clientId = row?.clientId?.trim?.();
    const auth0ClientId = row?.auth0ClientId?.trim?.();
    if (!tenantId || !clientId) {
      continue;
    }
    byClientId.set(clientId, { tenantId, clientId, auth0ClientId });
    if (auth0ClientId) {
      byAuth0ClientId.set(auth0ClientId, { tenantId, clientId, auth0ClientId });
    }
  }

  return {
    lookupByClientId(clientId) {
      return byClientId.get(String(clientId ?? "").trim()) ?? null;
    },
    lookupByAuth0ClientId(auth0ClientId) {
      return byAuth0ClientId.get(String(auth0ClientId ?? "").trim()) ?? null;
    },
    resolveTenantId(clientId, auth0ClientId) {
      const fromAuth0 = auth0ClientId
        ? byAuth0ClientId.get(String(auth0ClientId).trim())
        : null;
      if (fromAuth0?.tenantId) {
        return fromAuth0.tenantId;
      }
      const fromClient = clientId ? byClientId.get(String(clientId).trim()) : null;
      if (fromClient?.tenantId) {
        return fromClient.tenantId;
      }
      return env.DEFAULT_TENANT_ID?.trim() || null;
    },
    billingClientId(clientId, auth0ClientId) {
      const fromAuth0 = auth0ClientId
        ? byAuth0ClientId.get(String(auth0ClientId).trim())
        : null;
      if (fromAuth0?.clientId) {
        return fromAuth0.clientId;
      }
      const fromClient = clientId ? byClientId.get(String(clientId).trim()) : null;
      if (fromClient?.clientId) {
        return fromClient.clientId;
      }
      return String(clientId ?? "").trim() || null;
    },
  };
}
