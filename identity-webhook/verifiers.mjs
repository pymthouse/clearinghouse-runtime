/**
 * End-user auth verifiers for the identity webhook.
 *
 * A verifier resolves an end-user credential (from the webhook body's
 * Authorization) into a UsageIdentity:
 *   { issuer, client_id, usage_subject, usage_subject_type }
 *
 * - createApiKeyVerifier: resolves `sk_…` keys via a caller-supplied lookup.
 * - createOidcVerifier:   verifies a JWT bearer against an OIDC issuer's JWKS (jose),
 *   or exchanges a composite `app_*.pmth_*` API key via RFC 8693 then verifies.
 * - createEndUserVerifierFromEnv: picks exactly one verifier via IDENTITY_AUTH_MODE.
 */
import { createHash } from "node:crypto";
import { createLocalJWKSet, createRemoteJWKSet, jwtVerify } from "jose";
import { bearerToken, WebhookError } from "./protocol.mjs";
import { loadApiKeyStore } from "./keys.mjs";

export const IDENTITY_AUTH_MODES = ["api_key", "oidc"];

const GRANT_TYPE_TOKEN_EXCHANGE =
  "urn:ietf:params:oauth:grant-type:token-exchange";
const SUBJECT_ACCESS_TOKEN_TYPE =
  "urn:ietf:params:oauth:token-type:access_token";
const COMPOSITE_CLIENT_ID_RE = /^app_[a-z0-9]+$/;
const COMPOSITE_CACHE_MAX_TTL_SECONDS = 60;
const ALLOWED_JWT_ALGS = ["RS256"];

function nowSeconds() {
  return Math.floor(Date.now() / 1000);
}

/**
 * Split `app_<clientId>.pmth_<key>` into parts. Returns null when not composite.
 */
export function splitCompositeApiKey(token) {
  const trimmed = (token ?? "").trim();
  const dot = trimmed.indexOf(".");
  if (dot <= 0 || trimmed.indexOf(".", dot + 1) !== -1) {
    return null;
  }
  const publicClientId = trimmed.slice(0, dot);
  const apiKey = trimmed.slice(dot + 1);
  if (!COMPOSITE_CLIENT_ID_RE.test(publicClientId)) {
    return null;
  }
  if (!apiKey.startsWith("pmth_") || apiKey.startsWith("pmth_cs_")) {
    return null;
  }
  return { publicClientId, apiKey };
}

function isLoopbackHost(hostname) {
  const host = (hostname ?? "").toLowerCase();
  return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "[::1]";
}

/**
 * Validate OIDC_TOKEN_EXCHANGE_BASE_URL: HTTPS required except loopback.
 * @param {string} baseUrl
 * @returns {string} normalized origin (no trailing slash)
 */
export function normalizeTokenExchangeBaseUrl(baseUrl) {
  const trimmed = (baseUrl ?? "").trim().replace(/\/$/, "");
  if (!trimmed) {
    throw new Error("tokenExchangeBaseUrl is required for composite API key exchange");
  }
  let url;
  try {
    url = new URL(trimmed);
  } catch {
    throw new Error(`tokenExchangeBaseUrl is not a valid URL: ${trimmed}`);
  }
  if (url.protocol === "https:") {
    return url.origin;
  }
  if (url.protocol === "http:" && isLoopbackHost(url.hostname)) {
    return url.origin;
  }
  throw new Error(
    `tokenExchangeBaseUrl must be https (or http on loopback); got ${url.protocol}//${url.hostname}`,
  );
}

function sha256Hex(value) {
  return createHash("sha256").update(value).digest("hex");
}

/**
 * API-key verifier. `resolveApiKey(token)` returns
 * { userId, clientId?, usageSubjectType? } or null.
 */
export function createApiKeyVerifier({
  issuer,
  resolveApiKey,
  apiKeyPrefix = "sk_",
  defaultClientId = "demo-client",
  defaultUsageSubjectType = "api_key_user",
  expiryTtlSeconds = 60,
}) {
  if (!issuer) {
    throw new Error("createApiKeyVerifier: issuer is required");
  }
  if (typeof resolveApiKey !== "function") {
    throw new Error("createApiKeyVerifier: resolveApiKey is required");
  }
  return {
    kind: "api_key",
    verify: async ({ authorization }) => {
      const token = bearerToken(authorization);
      if (apiKeyPrefix && !token.startsWith(apiKeyPrefix)) {
        throw new WebhookError("invalid api key", { status: 401, code: "invalid_api_key" });
      }
      const resolved = await resolveApiKey(token);
      if (!resolved?.userId) {
        throw new WebhookError("invalid api key", { status: 401, code: "invalid_api_key" });
      }
      const identity = {
        issuer,
        client_id: resolved.clientId ?? defaultClientId,
        usage_subject: resolved.userId,
        usage_subject_type: resolved.usageSubjectType ?? defaultUsageSubjectType,
      };
      return { identity, expiry: nowSeconds() + expiryTtlSeconds, raw: resolved };
    },
  };
}

/**
 * Resolve `jwks_uri` via OIDC Discovery (issuer-relative
 * `/.well-known/openid-configuration`), matching oauth4webapi / builder-sdk.
 *
 * Example: issuer `https://staging.pymthouse.com/api/v1/oidc` →
 * discovery advertises `jwks_uri` `https://staging.pymthouse.com/api/v1/oidc/jwks`.
 *
 * @param {string} jwtIssuer
 * @param {{ fetchImpl?: typeof fetch }} [options]
 * @returns {Promise<string>}
 */
export async function discoverJwksUri(jwtIssuer, options = {}) {
  const fetchImpl = options.fetchImpl ?? fetch;
  const base = jwtIssuer.replace(/\/$/, "");
  const url = `${base}/.well-known/openid-configuration`;
  let response;
  try {
    response = await fetchImpl(url);
  } catch (err) {
    throw new Error(
      `OIDC discovery request failed (${url}): ${err instanceof Error ? err.message : err}`,
    );
  }
  if (!response.ok) {
    throw new Error(`OIDC discovery failed: expected 200 from ${url}, got ${response.status}`);
  }
  let doc;
  try {
    doc = await response.json();
  } catch {
    throw new Error(`OIDC discovery response is not JSON (${url})`);
  }
  if (!doc || typeof doc.jwks_uri !== "string" || !doc.jwks_uri.trim()) {
    throw new Error(`OIDC discovery document missing jwks_uri (${url})`);
  }
  return doc.jwks_uri.trim();
}

/**
 * Build a jose key resolver: explicit `jwks`, explicit `jwksUri`, or lazy OIDC
 * discovery of `jwks_uri` from `{issuer}/.well-known/openid-configuration`.
 *
 * When `fetchImpl` is set (tests / custom HTTP), JWKS is loaded through it into
 * a local keyset. Otherwise jose's createRemoteJWKSet fetches and caches.
 */
function createOidcKeyResolver({ jwks, jwksUri, jwtIssuer, fetchImpl }) {
  if (jwks) {
    return jwks;
  }

  let remote;
  let resolving;
  return async (protectedHeader, token) => {
    if (!remote) {
      resolving ??= (async () => {
        const uri = jwksUri ?? (await discoverJwksUri(jwtIssuer, { fetchImpl }));
        if (fetchImpl) {
          const response = await fetchImpl(uri);
          if (!response.ok) {
            throw new Error(
              `Expected 200 OK from the JSON Web Key Set HTTP response (${uri}) [${response.status}]`,
            );
          }
          const doc = await response.json();
          return createLocalJWKSet(doc);
        }
        return createRemoteJWKSet(new URL(uri));
      })();
      remote = await resolving;
    }
    return remote(protectedHeader, token);
  };
}

function mapVerifiedPayloadToIdentity({
  payload,
  identityIssuer,
  clientClaim,
  subjectClaim,
  subjectTypeValue,
  jwtAudience,
  requiredScopes,
}) {
  if (requiredScopes.length) {
    const granted = new Set(
      String(payload.scope ?? payload.scp ?? "")
        .split(/[\s,]+/)
        .filter(Boolean),
    );
    const missing = requiredScopes.filter((s) => !granted.has(s));
    if (missing.length) {
      throw new WebhookError(`missing required scope(s): ${missing.join(", ")}`, {
        status: 403,
        code: "insufficient_scope",
      });
    }
  }

  const usageSubject = payload[subjectClaim];
  if (!usageSubject) {
    throw new WebhookError(`token missing ${subjectClaim} claim`, {
      status: 401,
      code: "invalid_token",
    });
  }

  const identity = {
    issuer: identityIssuer,
    client_id: String(payload[clientClaim] ?? jwtAudience),
    usage_subject: String(usageSubject),
    usage_subject_type: subjectTypeValue,
  };
  const expiry = typeof payload.exp === "number" ? payload.exp : nowSeconds() + 60;
  return { identity, expiry, raw: payload };
}

async function verifyJwtBearer({
  token,
  keyset,
  jwtIssuer,
  jwtAudience,
  identityIssuer,
  clientClaim,
  subjectClaim,
  subjectTypeValue,
  requiredScopes,
}) {
  let payload;
  try {
    ({ payload } = await jwtVerify(token, keyset, {
      issuer: jwtIssuer,
      audience: jwtAudience,
      algorithms: ALLOWED_JWT_ALGS,
    }));
  } catch (err) {
    console.warn(`oidc verification failed: ${err.message}`);
    throw new WebhookError("oidc verification failed", {
      status: 401,
      code: "invalid_token",
    });
  }

  return mapVerifiedPayloadToIdentity({
    payload,
    identityIssuer,
    clientClaim,
    subjectClaim,
    subjectTypeValue,
    jwtAudience,
    requiredScopes,
  });
}

function createCompositeExchangeCache() {
  /** @type {Map<string, { expiresAt: number, result?: any, inflight?: Promise<any> }>} */
  const cache = new Map();

  return {
    get(key) {
      const entry = cache.get(key);
      if (!entry) {
        return null;
      }
      if (entry.result && entry.expiresAt > nowSeconds()) {
        return entry.result;
      }
      if (entry.inflight) {
        return entry.inflight;
      }
      cache.delete(key);
      return null;
    },
    setInflight(key, promise) {
      cache.set(key, {
        expiresAt: nowSeconds() + COMPOSITE_CACHE_MAX_TTL_SECONDS,
        inflight: promise,
      });
    },
    setResult(key, result, ttlSeconds) {
      const ttl = Math.max(1, Math.min(ttlSeconds, COMPOSITE_CACHE_MAX_TTL_SECONDS));
      cache.set(key, { expiresAt: nowSeconds() + ttl, result });
    },
    clear(key) {
      cache.delete(key);
    },
  };
}

async function exchangeCompositeApiKey({
  exchangeBaseUrl,
  publicClientId,
  apiKey,
  jwtAudience,
  m2mClientId,
  m2mClientSecret,
  fetchImpl,
}) {
  const url = `${exchangeBaseUrl}/api/v1/apps/${encodeURIComponent(publicClientId)}/oidc/token`;
  const body = new URLSearchParams({
    grant_type: GRANT_TYPE_TOKEN_EXCHANGE,
    subject_token: apiKey,
    subject_token_type: SUBJECT_ACCESS_TOKEN_TYPE,
    requested_token_type: SUBJECT_ACCESS_TOKEN_TYPE,
    audience: jwtAudience,
  });

  const headers = {
    "content-type": "application/x-www-form-urlencoded",
    accept: "application/json",
  };
  if (m2mClientId && m2mClientSecret) {
    headers.authorization = `Basic ${Buffer.from(`${m2mClientId}:${m2mClientSecret}`).toString("base64")}`;
  }

  let response;
  try {
    response = await fetchImpl(url, {
      method: "POST",
      headers,
      body: body.toString(),
    });
  } catch (err) {
    const keyHashPrefix = sha256Hex(apiKey).slice(0, 12);
    console.warn(
      `composite api key exchange request failed key_hash=${keyHashPrefix}: ${err instanceof Error ? err.message : err}`,
    );
    throw new WebhookError("token exchange failed", {
      status: 401,
      code: "invalid_token",
    });
  }

  let payload;
  try {
    payload = await response.json();
  } catch {
    payload = null;
  }

  if (!response.ok) {
    const correlationId =
      payload && typeof payload.correlation_id === "string" ? payload.correlation_id : "";
    const keyHashPrefix = sha256Hex(apiKey).slice(0, 12);
    console.warn(
      `composite api key exchange rejected status=${response.status} key_hash=${keyHashPrefix}` +
        (correlationId ? ` correlation_id=${correlationId}` : ""),
    );
    throw new WebhookError("token exchange failed", {
      status: 401,
      code: "invalid_token",
    });
  }

  const accessToken =
    payload && typeof payload.access_token === "string" ? payload.access_token.trim() : "";
  if (!accessToken) {
    throw new WebhookError("token exchange returned no access_token", {
      status: 401,
      code: "invalid_token",
    });
  }
  return accessToken;
}

/**
 * OIDC/JWT verifier (bring-your-own OAuth). Validates a `Bearer <jwt>` against
 * `jwtIssuer`/`jwtAudience` using the issuer's JWKS (auto-cached & refreshed by
 * jose's createRemoteJWKSet).
 *
 * Also accepts composite `Bearer app_<clientId>.pmth_<key>` when
 * `tokenExchangeBaseUrl` is configured: RFC 8693 exchange at
 * `/api/v1/apps/{clientId}/oidc/token`, then the same JWT verification path.
 *
 * JWKS resolution order:
 * 1. `jwks` (injected keyset, for tests)
 * 2. `jwksUri` (explicit override, e.g. OIDC_JWKS_URI)
 * 3. OIDC Discovery: `{jwtIssuer}/.well-known/openid-configuration` → `jwks_uri`
 */
export function createOidcVerifier({
  jwtIssuer,
  jwtAudience,
  jwks,
  jwksUri,
  issuer,
  clientClaim = "azp",
  subjectClaim = "sub",
  subjectTypeValue = "oidc_user",
  requiredScopes = [],
  fetchImpl,
  tokenExchangeBaseUrl,
  exchangeM2mClientId,
  exchangeM2mClientSecret,
}) {
  if (!jwtIssuer) {
    throw new Error("createOidcVerifier: jwtIssuer is required");
  }
  if (!jwtAudience) {
    throw new Error("createOidcVerifier: jwtAudience is required");
  }
  const keyset = createOidcKeyResolver({
    jwks,
    jwksUri,
    jwtIssuer,
    fetchImpl,
  });
  const identityIssuer = issuer ?? jwtIssuer;
  const http = fetchImpl ?? fetch;
  const exchangeOrigin = tokenExchangeBaseUrl
    ? normalizeTokenExchangeBaseUrl(tokenExchangeBaseUrl)
    : null;
  const m2mId = (exchangeM2mClientId ?? "").trim();
  const m2mSecret = (exchangeM2mClientSecret ?? "").trim();
  if ((m2mId && !m2mSecret) || (!m2mId && m2mSecret)) {
    throw new Error(
      "createOidcVerifier: exchangeM2mClientId and exchangeM2mClientSecret must both be set or both omitted",
    );
  }
  const exchangeCache = createCompositeExchangeCache();

  async function verifyJwt(token) {
    return verifyJwtBearer({
      token,
      keyset,
      jwtIssuer,
      jwtAudience,
      identityIssuer,
      clientClaim,
      subjectClaim,
      subjectTypeValue,
      requiredScopes,
    });
  }

  async function verifyComposite(compositeToken, parts) {
    if (!exchangeOrigin) {
      throw new WebhookError("composite api key exchange is not configured", {
        status: 401,
        code: "invalid_token",
      });
    }

    const cacheKey = sha256Hex(compositeToken);
    const cached = exchangeCache.get(cacheKey);
    if (cached) {
      return cached instanceof Promise ? cached : cached;
    }

    const inflight = (async () => {
      const accessToken = await exchangeCompositeApiKey({
        exchangeBaseUrl: exchangeOrigin,
        publicClientId: parts.publicClientId,
        apiKey: parts.apiKey,
        jwtAudience,
        m2mClientId: m2mId,
        m2mClientSecret: m2mSecret,
        fetchImpl: http,
      });
      const verified = await verifyJwt(accessToken);
      if (verified.identity.client_id !== parts.publicClientId) {
        throw new WebhookError("minted token client_id does not match credential prefix", {
          status: 401,
          code: "invalid_token",
        });
      }
      const ttl = Math.max(1, verified.expiry - nowSeconds());
      exchangeCache.setResult(cacheKey, verified, ttl);
      return verified;
    })().catch((err) => {
      exchangeCache.clear(cacheKey);
      throw err;
    });

    exchangeCache.setInflight(cacheKey, inflight);
    return inflight;
  }

  return {
    kind: "oidc",
    verify: async ({ authorization }) => {
      const token = bearerToken(authorization);
      if (!token) {
        throw new WebhookError("not a JWT", { status: 401, code: "invalid_token" });
      }

      const composite = splitCompositeApiKey(token);
      if (composite) {
        return verifyComposite(token, composite);
      }

      if (token.split(".").length !== 3) {
        throw new WebhookError("not a JWT", { status: 401, code: "invalid_token" });
      }

      return verifyJwt(token);
    },
  };
}

function envTrim(env, name) {
  return env[name]?.trim() || "";
}

function envOptional(env, name, fallback) {
  return envTrim(env, name) || fallback;
}

/**
 * Build the end-user verifier from env. IDENTITY_AUTH_MODE selects exactly one
 * verifier — no fallback between OIDC and API-key paths.
 */
export function createEndUserVerifierFromEnv(env) {
  const issuer = envTrim(env, "IDENTITY_ISSUER");
  if (!issuer) {
    throw new Error("IDENTITY_ISSUER is required");
  }

  const mode = envTrim(env, "IDENTITY_AUTH_MODE");
  if (!IDENTITY_AUTH_MODES.includes(mode)) {
    throw new Error(`IDENTITY_AUTH_MODE is required (${IDENTITY_AUTH_MODES.join(" | ")})`);
  }

  if (mode === "api_key") {
    if (!envTrim(env, "DEMO_API_KEY") && !envTrim(env, "DEMO_API_KEYS")) {
      throw new Error("api_key mode requires DEMO_API_KEY and/or DEMO_API_KEYS");
    }
    const keyStore = loadApiKeyStore(env);
    return createApiKeyVerifier({
      issuer,
      apiKeyPrefix: envOptional(env, "API_KEY_PREFIX", "sk_"),
      defaultClientId: envOptional(env, "DEMO_CLIENT_ID", "demo-client"),
      defaultUsageSubjectType: envOptional(env, "USAGE_SUBJECT_TYPE", "api_key_user"),
      resolveApiKey: async (apiKey) => keyStore.get(apiKey) ?? null,
    });
  }

  if (!envTrim(env, "OIDC_ISSUER")) {
    throw new Error("oidc mode requires OIDC_ISSUER");
  }
  if (!envTrim(env, "OIDC_AUDIENCE")) {
    throw new Error("oidc mode requires OIDC_AUDIENCE");
  }

  return createOidcVerifier({
    jwtIssuer: envTrim(env, "OIDC_ISSUER"),
    jwtAudience: envTrim(env, "OIDC_AUDIENCE"),
    jwksUri: envTrim(env, "OIDC_JWKS_URI") || undefined,
    issuer,
    clientClaim: envOptional(env, "OIDC_CLIENT_CLAIM", "azp"),
    subjectClaim: envOptional(env, "OIDC_SUBJECT_CLAIM", "sub"),
    subjectTypeValue: envOptional(env, "OIDC_SUBJECT_TYPE", "oidc_user"),
    requiredScopes: (envTrim(env, "OIDC_REQUIRED_SCOPES") || "")
      .split(/[\s,]+/)
      .filter(Boolean),
    tokenExchangeBaseUrl: envTrim(env, "OIDC_TOKEN_EXCHANGE_BASE_URL") || undefined,
    exchangeM2mClientId: envTrim(env, "OIDC_EXCHANGE_M2M_CLIENT_ID") || undefined,
    exchangeM2mClientSecret: envTrim(env, "OIDC_EXCHANGE_M2M_CLIENT_SECRET") || undefined,
  });
}
