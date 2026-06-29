// Synchronous "ensure the customer exists before service" gate for the identity webhook.
//
// builder-sdk invokes config.afterVerify after the end-user identity is verified and
// before the signer receives the auth_id. We use it to ensure the OpenMeter customer
// (keyed clientId:externalUserId, the builder-sdk format) exists in tenant-admin BEFORE
// any ticket is signed — replacing the collector's old reactive, post-hoc upsert.
//
// Successful ensures are cached in-memory (TTL) so steady-state signing traffic does not
// depend on tenant-admin. By default a failed ensure blocks signing (fail-closed), which
// honours "know the customer before providing service"; set ENSURE_FAIL_OPEN=true to
// degrade to allow-on-error instead.

const DEFAULT_TTL_SECONDS = 600;
const DEFAULT_TIMEOUT_MS = 5000;
const MAX_CACHE_ENTRIES = 50_000;

export function createEnsureCustomerHook(options) {
  const url = (options.url ?? "").trim();
  if (!url) {
    throw new Error("createEnsureCustomerHook: url is required");
  }
  const secret = (options.secret ?? "").trim();
  const ttlMs = Math.max(1, options.ttlSeconds ?? DEFAULT_TTL_SECONDS) * 1000;
  const timeoutMs = Math.max(1, options.timeoutMs ?? DEFAULT_TIMEOUT_MS);
  const failOpen = options.failOpen === true;
  const fetchImpl = options.fetch ?? globalThis.fetch;

  const cache = new Map(); // key -> expiry epoch ms

  function isCached(key) {
    const expiry = cache.get(key);
    if (expiry === undefined) {
      return false;
    }
    if (expiry <= Date.now()) {
      cache.delete(key);
      return false;
    }
    return true;
  }

  function remember(key) {
    cache.set(key, Date.now() + ttlMs);
    if (cache.size <= MAX_CACHE_ENTRIES) {
      return;
    }
    for (const [existingKey, expiry] of cache) {
      if (expiry <= Date.now()) {
        cache.delete(existingKey);
      }
    }
    while (cache.size > MAX_CACHE_ENTRIES) {
      const oldest = cache.keys().next().value;
      if (oldest === undefined) {
        break;
      }
      cache.delete(oldest);
    }
  }

  return async function ensureCustomer({ identity }) {
    const clientId = (identity?.client_id ?? "").trim();
    const externalUserId = (identity?.usage_subject ?? "").trim();
    if (!clientId || !externalUserId) {
      // Identity is already validated upstream; if a segment is missing there is
      // nothing to ensure (and the verifier would have rejected it).
      return;
    }
    const key = `${clientId}:${externalUserId}`;
    if (isCached(key)) {
      return;
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    let response;
    try {
      const headers = { "Content-Type": "application/json" };
      if (secret) {
        headers["X-Internal-Secret"] = secret;
      }
      response = await fetchImpl(url, {
        method: "POST",
        headers,
        body: JSON.stringify({ clientId, externalUserId }),
        signal: controller.signal,
      });
    } catch (err) {
      return handleFailure(key, `customer ensure request failed: ${err.message}`, failOpen);
    } finally {
      clearTimeout(timer);
    }

    if (!response.ok) {
      let detail = "";
      try {
        detail = (await response.text()).slice(0, 300);
      } catch {
        // ignore body read errors
      }
      return handleFailure(key, `customer ensure returned ${response.status}: ${detail}`, failOpen);
    }
    remember(key);
  };
}

function handleFailure(key, message, failOpen) {
  if (failOpen) {
    console.warn(`identity-webhook: ${message} (fail-open, allowing ${key})`);
    return;
  }
  throw new Error(message);
}
