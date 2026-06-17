export const DEFAULT_KONNECT_METERING_URL = "https://us.api.konghq.com/v3";

export const KONNECT_REGIONS = ["us", "eu", "au", "sg"] as const;

export function isKonnectApiKey(apiKey: string | undefined): boolean {
  const key = apiKey?.trim() ?? "";
  return key.startsWith("kpat_") || key.startsWith("spat_");
}

export function isKonnectMeteringUrl(url: string, apiKey?: string): boolean {
  if (/konghq\.com/i.test(url)) {
    return true;
  }
  return isKonnectApiKey(apiKey);
}

/** Normalize OPENMETER_URL to the Konnect API root (…/v3). */
export function normalizeKonnectMeteringUrl(url: string): string {
  let base = url.trim().replace(/\/$/, "");
  if (base.endsWith("/events")) {
    base = base.slice(0, -"/events".length);
  }
  if (base.endsWith("/openmeter")) {
    base = base.slice(0, -"/openmeter".length);
  }
  return base;
}

/** Ingest endpoint for CloudEvents (collector / direct POST). */
export function konnectIngestUrl(baseUrl: string): string {
  return `${normalizeKonnectMeteringUrl(baseUrl)}/openmeter/events`;
}

/** Livepeer convention: Auth0 azp + sub */
export function buildCustomerKey(
  clientId: string,
  externalUserId: string,
): string {
  return `${clientId}:${externalUserId}`;
}
