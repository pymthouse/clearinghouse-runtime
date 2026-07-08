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
    requiredScopes: (envTrim(env, "OIDC_REQUIRED_SCOPES") || "")
      .split(/[\s,]+/)
      .filter(Boolean),
    jwksUri: envTrim(env, "OIDC_JWKS_URI") || undefined,
  });
}

/**
 * Full webhook config for handleAuthorize from legacy env.
 *
 * @param {NodeJS.ProcessEnv | Record<string, string | undefined>} env
 * @param {{ jwtIssuer?: string }} [options]
 */
export function createLegacyWebhookConfigFromEnv(env, options = {}) {
  const webhookSecret = envTrim(env, "WEBHOOK_SECRET");
  if (!webhookSecret) {
    throw new Error("WEBHOOK_SECRET is required");
  }

  return {
    webhookSecret,
    endUserAuth: createLegacyOidcVerifierFromEnv(env, options),
  };
}
