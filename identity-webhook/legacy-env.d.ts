import type { EndUserAuthVerifier, RemoteSignerWebhookConfig } from "./protocol.js";

export function defaultSignerWebhookJwtAudience(jwtIssuer: string): string;

export function createLegacyOidcVerifierFromEnv(
  env: NodeJS.ProcessEnv | Record<string, string | undefined>,
  options?: { jwtIssuer?: string },
): EndUserAuthVerifier;

export function createLegacyWebhookConfigFromEnv(
  env: NodeJS.ProcessEnv | Record<string, string | undefined>,
  options?: { jwtIssuer?: string },
): RemoteSignerWebhookConfig;
