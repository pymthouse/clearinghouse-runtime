import {
  DEFAULT_KONNECT_METERING_URL,
  isKonnectMeteringUrl,
  normalizeKonnectMeteringUrl,
} from "@pymthouse/konnect-metering";
import { KonnectAdmin } from "./adapters/konnect.adapter.js";
import { SelfHostedAdmin } from "./adapters/selfhosted.adapter.js";
import type { OpenMeterAdmin } from "./port.js";

export type CreateAdminOptions = {
  baseUrl: string;
  apiKey?: string;
};

export function createAdmin(opts: CreateAdminOptions): OpenMeterAdmin {
  const baseUrl = opts.baseUrl.trim();
  const apiKey = opts.apiKey?.trim();

  if (isKonnectMeteringUrl(baseUrl, apiKey)) {
    if (!apiKey) {
      throw new Error(
        "OPENMETER_API_KEY is required for Konnect Metering & Billing — use a Konnect Personal Access Token (kpat_…)",
      );
    }
    const konnectBase = normalizeKonnectMeteringUrl(
      /konghq\.com/i.test(baseUrl) ? baseUrl : DEFAULT_KONNECT_METERING_URL,
    );
    return new KonnectAdmin({ baseUrl: konnectBase, apiKey });
  }

  return new SelfHostedAdmin({ baseUrl, apiKey });
}

export { KonnectAdmin, SelfHostedAdmin };
