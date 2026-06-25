import { readFileSync } from "node:fs";

function loadClientTenantMap(env) {
  const map = new Map();
  const mapFromEnv = env.TENANT_CLIENT_MAP?.trim();
  if (mapFromEnv) {
    try {
      const parsed = JSON.parse(mapFromEnv);
      if (parsed && typeof parsed === "object") {
        for (const [clientId, tenantId] of Object.entries(parsed)) {
          if (String(clientId).trim() && String(tenantId).trim()) {
            map.set(String(clientId).trim(), String(tenantId).trim());
          }
        }
      }
    } catch {
      throw new Error("TENANT_CLIENT_MAP must be valid JSON");
    }
  }

  const registryPath = env.TENANT_REGISTRY_PATH?.trim();
  if (registryPath) {
    try {
      const raw = readFileSync(registryPath, "utf8");
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        for (const row of parsed) {
          const clientId = row?.clientId?.trim?.();
          const tenantId = row?.tenantId?.trim?.();
          if (clientId && tenantId) {
            map.set(clientId, tenantId);
          }
        }
      }
    } catch {
      // Optional mapping source; keep env/default-only behavior on read errors.
    }
  }

  return map;
}

/**
 * Load demo API keys from env for the in-compose identity webhook.
 *
 * DEMO_API_KEY + DEMO_CLIENT_ID + DEMO_USER_ID define one key.
 * Optional DEMO_API_KEYS JSON map for multiple keys:
 *   {"sk_other":{"tenantId":"acme","clientId":"app-b","userId":"user-b"}}
 */
export function loadApiKeyStore(env) {
  const store = new Map();
  const clientTenantMap = loadClientTenantMap(env);

  const primaryKey = env.DEMO_API_KEY?.trim();
  if (primaryKey) {
    store.set(primaryKey, {
      tenantId:
        env.DEMO_TENANT_ID?.trim() ||
        clientTenantMap.get(env.DEMO_CLIENT_ID?.trim() || "demo-client") ||
        env.DEFAULT_TENANT_ID?.trim() ||
        "default",
      clientId: env.DEMO_CLIENT_ID?.trim() || "demo-client",
      userId: env.DEMO_USER_ID?.trim() || "demo-user",
      usageSubjectType: env.USAGE_SUBJECT_TYPE?.trim() || "api_key_user",
    });
  }

  const extra = env.DEMO_API_KEYS?.trim();
  if (extra) {
    let parsed;
    try {
      parsed = JSON.parse(extra);
    } catch {
      throw new Error("DEMO_API_KEYS must be valid JSON");
    }
    if (parsed && typeof parsed === "object") {
      for (const [apiKey, entry] of Object.entries(parsed)) {
        if (!apiKey?.trim() || !entry || typeof entry !== "object") {
          continue;
        }
        const userId = entry.userId?.trim() || entry.user_id?.trim();
        if (!userId) {
          continue;
        }
        store.set(apiKey.trim(), {
          tenantId:
            entry.tenantId?.trim() ||
            entry.tenant_id?.trim() ||
            clientTenantMap.get(entry.clientId?.trim() || entry.client_id?.trim() || "") ||
            env.DEFAULT_TENANT_ID?.trim() ||
            "default",
          clientId: entry.clientId?.trim() || entry.client_id?.trim() || "demo-client",
          userId,
          usageSubjectType:
            entry.usageSubjectType?.trim() ||
            entry.usage_subject_type?.trim() ||
            "api_key_user",
        });
      }
    }
  }

  if (store.size === 0) {
    throw new Error("DEMO_API_KEY or DEMO_API_KEYS is required");
  }

  return store;
}
