/**
 * Map pymthouse / builder-sdk legacy env vars to identity-webhook OIDC verifier config.
 *
 * Preserves pymthouse defaults from src/app/webhooks/remote-signer/route.ts:
 *   CLAIM_CLIENT_ID=client_id, CLAIM_USAGE_SUBJECT=external_user_id,
 *   USAGE_SUBJECT_TYPE=external_user_id, JWT_AUDIENCE defaults to JWT_ISSUER.
 */
import { createOidcVerifier } from "./verifiers.mjs";

function envTrim(env, name) {
  return env[name]?.trim() || "";
}

function envOptional(env, name, fallback) {
  return envTrim(env, name) || fallback;
}

/** Strip trailing slashes from an issuer URL (builder-sdk audience default). */
export function defaultSignerWebhookJwtAudience(jwtIssuer) {
  let end = jwtIssuer.length;
  while (end > 0 && jwtIssuer[end - 1] === "/") {
    end -= 1;
  }
  return jwtIssuer.slice(0, end);
}

/**
 * Optional explicit JWKS override. When unset, createOidcVerifier resolves
 * `jwks_uri` via OIDC Discovery (`{issuer}/.well-known/openid-configuration`).
 *
 * @param {NodeJS.ProcessEnv | Record<string, string | undefined>} env
 */
export function resolveLegacyJwksUri(env) {
  return envTrim(env, "OIDC_JWKS_URI") || envTrim(env, "JWKS_URI") || undefined;
}

/**
 * Base URL for RFC 8693 app-scoped token exchange (composite API keys).
 * Prefer OIDC_TOKEN_EXCHANGE_BASE_URL; else NEXTAUTH_URL / public origin.
 *
 * @param {NodeJS.ProcessEnv | Record<string, string | undefined>} env
 */
export function resolveLegacyTokenExchangeBaseUrl(env) {
  return (
    envTrim(env, "OIDC_TOKEN_EXCHANGE_BASE_URL") ||
    envTrim(env, "NEXTAUTH_URL") ||
    undefined
  );
}

/**
 * Build an OIDC end-user verifier from legacy JWT_* / CLAIM_* env vars.
 *
 * @param {NodeJS.ProcessEnv | Record<string, string | undefined>} env
 * @param {{ jwtIssuer?: string }} [options] - optional issuer override (e.g. pymthouse getIssuer())
 */
export function createLegacyOidcVerifierFromEnv(env, options = {}) {
  const jwtIssuer = options.jwtIssuer?.trim() || envTrim(env, "JWT_ISSUER");
  if (!jwtIssuer) {
    throw new Error("JWT_ISSUER is required");
  }

  const jwtAudience =
    envTrim(env, "JWT_AUDIENCE") || defaultSignerWebhookJwtAudience(jwtIssuer);

  return createOidcVerifier({
    jwtIssuer,
    jwtAudience,
    issuer: jwtIssuer,
    clientClaim: envOptional(env, "CLAIM_CLIENT_ID", "client_id"),
    subjectClaim: envOptional(env, "CLAIM_USAGE_SUBJECT", "external_user_id"),
    subjectTypeValue: envOptional(env, "USAGE_SUBJECT_TYPE", "external_user_id"),
    requiredScopes: (envTrim(env, "OIDC_REQUIRED_SCOPES") || "sign:job")
      .split(/[\s,]+/)
      .filter(Boolean),
    jwksUri: resolveLegacyJwksUri(env),
    tokenExchangeBaseUrl: resolveLegacyTokenExchangeBaseUrl(env),
    exchangeM2mClientId: envTrim(env, "OIDC_EXCHANGE_M2M_CLIENT_ID") || undefined,
    exchangeM2mClientSecret: envTrim(env, "OIDC_EXCHANGE_M2M_CLIENT_SECRET") || undefined,
  });
}

/**
 * Full webhook config for handleAuthorize from legacy env.
 *
 * @param {NodeJS.ProcessEnv | Record<string, string | undefined>} env
 * @param {{ jwtIssuer?: string, checkBalance?: import("./protocol.js").BalanceCheck }} [options]
 *   `checkBalance` is passed through so consumers can enforce a live credit gate
 *   (see ./balance-gate.mjs) without re-implementing verifier wiring.
 */
export function createLegacyWebhookConfigFromEnv(env, options = {}) {
  const webhookSecret = envTrim(env, "WEBHOOK_SECRET");
  if (!webhookSecret) {
    throw new Error("WEBHOOK_SECRET is required");
  }

  const config = {
    webhookSecret,
    endUserAuth: createLegacyOidcVerifierFromEnv(env, options),
  };
  if (typeof options.checkBalance === "function") {
    config.checkBalance = options.checkBalance;
  }
  return config;
}
