import { OpenMeter } from "@openmeter/sdk";
import {
  bootstrapKonnectMetering,
  DEFAULT_KONNECT_METERING_URL,
  isKonnectMeteringUrl,
  normalizeKonnectMeteringUrl,
} from "./konnect-metering.js";
import {
  BILLABLE_USD_MICROS_METER,
  DEFAULT_BILLABLE_FEATURE_KEY,
  DEFAULT_TRIAL_FEATURE_KEY,
  OPENMETER_METER_DEFINITIONS,
} from "./meters.js";
import {
  billableFeatureKey,
  defaultPlanKey,
  getPricingConfig,
} from "./pricing.js";

async function waitForOpenMeterHealthy(
  baseUrl: string,
  attempts = 30,
): Promise<void> {
  for (let i = 0; i < attempts; i++) {
    try {
      const resp = await fetch(
        `${baseUrl.replace(/\/$/, "")}/api/v1/debug/metrics`,
      );
      if (resp.ok) {
        return;
      }
    } catch {
      /* retry */
    }
    await new Promise((r) => setTimeout(r, 2000));
  }
  throw new Error(`OpenMeter not healthy at ${baseUrl}`);
}

async function bootstrapLegacyOpenMeter(opts: {
  baseUrl: string;
  apiKey?: string;
  trialFeatureKey?: string;
}): Promise<void> {
  const baseUrl = opts.baseUrl.replace(/\/$/, "");
  await waitForOpenMeterHealthy(baseUrl);

  const client = opts.apiKey?.trim()
    ? new OpenMeter({ baseUrl, apiKey: opts.apiKey.trim() })
    : new OpenMeter({ baseUrl });

  const existing = await client.meters.list();

  for (const meter of OPENMETER_METER_DEFINITIONS) {
    const existingMeter = (existing || []).find((m) => m.slug === meter.slug);
    if (existingMeter) {
      const groupBy = existingMeter.groupBy ?? {};
      const hasPipelineGroupBy =
        "pipeline" in groupBy && "model_id" in groupBy;
      if (!hasPipelineGroupBy) {
        console.warn(
          `[openmeter] meter ${meter.slug} missing pipeline/model_id groupBy — recreate manually if needed`,
        );
      }
      continue;
    }
    await client.meters.create(meter);
    console.log(`[openmeter] created meter ${meter.slug}`);
  }

  const networkFeatureKey = opts.trialFeatureKey?.trim() || DEFAULT_TRIAL_FEATURE_KEY;
  const billableFeatureKeyResolved =
    billableFeatureKey() || DEFAULT_BILLABLE_FEATURE_KEY;

  for (const [key, name, meterSlug] of [
    [networkFeatureKey, "Network spend", "network_fee_usd_micros"] as const,
    [billableFeatureKeyResolved, "Billable spend", BILLABLE_USD_MICROS_METER] as const,
  ]) {
    try {
      const features = await client.features.list();
      const hasFeature = (features || []).some((f) => f.key === key);
      if (!hasFeature) {
        await client.features.create({
          key,
          name,
          meterSlug,
        });
        console.log(`[openmeter] created feature ${key}`);
      } else {
        console.log(`[openmeter] feature exists: ${key}`);
      }
    } catch (err) {
      console.warn(`[openmeter] feature bootstrap skipped for ${key}:`, err);
    }
  }

  console.log(
    "[openmeter] Default pay-per-use plan bootstrap is Konnect-only; self-hosted plan API skipped.",
  );
  console.log(
    `[openmeter] Target plan key when using Konnect: ${defaultPlanKey()}`,
  );
}

export async function bootstrapOpenMeter(opts: {
  baseUrl: string;
  apiKey?: string;
  trialFeatureKey?: string;
}): Promise<void> {
  const pricing = getPricingConfig();
  console.log(
    `[bootstrap] pricing: plan=${defaultPlanKey()} billable_feature=${billableFeatureKey()} trial_micros=${pricing.defaultTrialIncludedUsdMicros}`,
  );

  if (isKonnectMeteringUrl(opts.baseUrl, opts.apiKey)) {
    if (!opts.apiKey?.trim()) {
      throw new Error(
        "OPENMETER_API_KEY is required for Konnect Metering & Billing — use a Konnect Personal Access Token (kpat_…)",
      );
    }
    const baseUrl = normalizeKonnectMeteringUrl(
      /konghq\.com/i.test(opts.baseUrl)
        ? opts.baseUrl
        : DEFAULT_KONNECT_METERING_URL,
    );
    console.log("[bootstrap] provisioning Konnect Metering & Billing at", baseUrl);
    await bootstrapKonnectMetering({
      baseUrl,
      apiKey: opts.apiKey,
      trialFeatureKey: opts.trialFeatureKey,
    });
    return;
  }

  console.log("[bootstrap] provisioning OpenMeter at", opts.baseUrl);
  await bootstrapLegacyOpenMeter(opts);
}
