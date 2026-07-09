import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { SignJWT, createLocalJWKSet, exportJWK, generateKeyPair } from "jose";
import {
  createLegacyOidcVerifierFromEnv,
  createLegacyWebhookConfigFromEnv,
  defaultSignerWebhookJwtAudience,
  resolveLegacyJwksUri,
  resolveLegacyTokenExchangeBaseUrl,
} from "./legacy-env.mjs";
import { handleAuthorize } from "./protocol.mjs";

describe("defaultSignerWebhookJwtAudience", () => {
  it("strips trailing slashes", () => {
    assert.equal(
      defaultSignerWebhookJwtAudience("https://pymthouse.com/api/v1/oidc/"),
      "https://pymthouse.com/api/v1/oidc",
    );
  });
});

describe("resolveLegacyJwksUri", () => {
  it("returns undefined so createOidcVerifier uses OIDC discovery", () => {
    assert.equal(resolveLegacyJwksUri({}), undefined);
  });

  it("prefers OIDC_JWKS_URI over JWKS_URI", () => {
    assert.equal(
      resolveLegacyJwksUri({
        OIDC_JWKS_URI: "https://a.example/jwks",
        JWKS_URI: "https://b.example/jwks",
      }),
      "https://a.example/jwks",
    );
    assert.equal(
      resolveLegacyJwksUri({ JWKS_URI: "https://b.example/jwks" }),
      "https://b.example/jwks",
    );
  });
});

describe("resolveLegacyTokenExchangeBaseUrl", () => {
  it("prefers OIDC_TOKEN_EXCHANGE_BASE_URL over NEXTAUTH_URL", () => {
    assert.equal(
      resolveLegacyTokenExchangeBaseUrl({
        OIDC_TOKEN_EXCHANGE_BASE_URL: "https://staging.pymthouse.com",
        NEXTAUTH_URL: "http://localhost:3000",
      }),
      "https://staging.pymthouse.com",
    );
    assert.equal(
      resolveLegacyTokenExchangeBaseUrl({ NEXTAUTH_URL: "http://localhost:3000" }),
      "http://localhost:3000",
    );
  });
});

describe("createLegacyOidcVerifierFromEnv", () => {
  it("uses pymthouse claim defaults", () => {
    const verifier = createLegacyOidcVerifierFromEnv({
      JWT_ISSUER: "https://pymthouse.com/api/v1/oidc",
    });
    assert.equal(verifier.kind, "oidc");
  });

  it("defaults JWT_AUDIENCE to issuer without trailing slash", () => {
    const verifier = createLegacyOidcVerifierFromEnv({
      JWT_ISSUER: "https://pymthouse.com/api/v1/oidc/",
    });
    assert.equal(verifier.kind, "oidc");
  });

  it("accepts jwtIssuer override", () => {
    const verifier = createLegacyOidcVerifierFromEnv(
      {},
      { jwtIssuer: "https://override.example/oidc" },
    );
    assert.equal(verifier.kind, "oidc");
  });

  it("throws when JWT_ISSUER is missing", () => {
    assert.throws(
      () => createLegacyOidcVerifierFromEnv({}),
      /JWT_ISSUER is required/,
    );
  });
});

describe("createLegacyWebhookConfigFromEnv", () => {
  it("requires WEBHOOK_SECRET", () => {
    assert.throws(
      () =>
        createLegacyWebhookConfigFromEnv({
          JWT_ISSUER: "https://pymthouse.com/api/v1/oidc",
        }),
      /WEBHOOK_SECRET is required/,
    );
  });

  it("returns webhookSecret and endUserAuth", () => {
    const config = createLegacyWebhookConfigFromEnv({
      WEBHOOK_SECRET: "secret",
      JWT_ISSUER: "https://pymthouse.com/api/v1/oidc",
      CLAIM_CLIENT_ID: "client_id",
      CLAIM_USAGE_SUBJECT: "external_user_id",
      USAGE_SUBJECT_TYPE: "external_user_id",
    });
    assert.equal(config.webhookSecret, "secret");
    assert.equal(config.endUserAuth.kind, "oidc");
  });
});

describe("pymthouse embedded flow (handleAuthorize + legacy config)", () => {
  const ISSUER = "https://pymthouse.com/api/v1/oidc";
  const SECRET = "dev-webhook-secret";

  async function setupPymthouseJwt() {
    const { publicKey, privateKey } = await generateKeyPair("RS256");
    const jwk = await exportJWK(publicKey);
    jwk.kid = "pymthouse-test";
    jwk.alg = "RS256";
    jwk.use = "sig";
    const jwks = createLocalJWKSet({ keys: [jwk] });
    const token = await new SignJWT({
      client_id: "app_abc123",
      external_user_id: "user-456",
      scope: "sign:job",
    })
      .setProtectedHeader({ alg: "RS256", kid: "pymthouse-test" })
      .setIssuer(ISSUER)
      .setAudience(defaultSignerWebhookJwtAudience(ISSUER))
      .setIssuedAt()
      .setExpirationTime("5m")
      .sign(privateKey);
    return { token, jwks };
  }

  it("authorizes a pymthouse-style JWT end-to-end", async () => {
    const { token, jwks } = await setupPymthouseJwt();
    const { createOidcVerifier } = await import("./verifiers.mjs");
    const config = {
      webhookSecret: SECRET,
      endUserAuth: createOidcVerifier({
        jwtIssuer: ISSUER,
        jwtAudience: defaultSignerWebhookJwtAudience(ISSUER),
        issuer: ISSUER,
        jwks,
        clientClaim: "client_id",
        subjectClaim: "external_user_id",
        subjectTypeValue: "external_user_id",
        requiredScopes: ["sign:job"],
      }),
    };

    const request = new Request("http://localhost/webhooks/remote-signer", {
      method: "POST",
      headers: {
        authorization: `Bearer ${SECRET}`,
        "content-type": "application/json",
      },
      body: JSON.stringify({
        headers: { Authorization: [`Bearer ${token}`] },
      }),
    });

    const response = await handleAuthorize(request, config);
    assert.equal(response.status, 200);
    const body = await response.json();
    assert.equal(body.status, 200);
    assert.equal(body.auth_id, "app_abc123:user-456");
    assert.equal(body.identity.client_id, "app_abc123");
    assert.equal(body.identity.usage_subject, "user-456");
    assert.equal(body.identity.usage_subject_type, "external_user_id");
  });

  it("authorizes a composite app_*.pmth_* via mocked exchange", async () => {
    const { token, jwks } = await setupPymthouseJwt();
    const { createOidcVerifier } = await import("./verifiers.mjs");
    const clientId = "app_abc123";
    const fetchImpl = async (input) => {
      const url = String(input);
      assert.ok(url.includes(`/api/v1/apps/${clientId}/oidc/token`));
      return Response.json({ access_token: token, expires_in: 300 });
    };
    const config = {
      webhookSecret: SECRET,
      endUserAuth: createOidcVerifier({
        jwtIssuer: ISSUER,
        jwtAudience: defaultSignerWebhookJwtAudience(ISSUER),
        issuer: ISSUER,
        jwks,
        clientClaim: "client_id",
        subjectClaim: "external_user_id",
        subjectTypeValue: "external_user_id",
        requiredScopes: ["sign:job"],
        tokenExchangeBaseUrl: "http://localhost:3000",
        fetchImpl,
      }),
    };

    const request = new Request("http://localhost/webhooks/remote-signer", {
      method: "POST",
      headers: {
        authorization: `Bearer ${SECRET}`,
        "content-type": "application/json",
      },
      body: JSON.stringify({
        headers: { Authorization: [`Bearer ${clientId}.pmth_deadbeef`] },
      }),
    });

    const response = await handleAuthorize(request, config);
    assert.equal(response.status, 200);
    const body = await response.json();
    assert.equal(body.status, 200);
    assert.equal(body.auth_id, "app_abc123:user-456");
  });
});
