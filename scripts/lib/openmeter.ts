import { OpenMeter } from "@openmeter/sdk";
import {
  bootstrapKonnectMetering,
  DEFAULT_KONNECT_METERING_URL,
  isKonnectMeteringUrl,
  normalizeKonnectMeteringUrl,
} from "./konnect-metering.js";
import {
  DEFAULT_TRIAL_FEATURE_KEY,
  OPENMETER_METER_DEFINITIONS,
} from "./meters.js";

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

  const featureKey = opts.trialFeatureKey?.trim() || DEFAULT_TRIAL_FEATURE_KEY;
  try {
    const features = await client.features.list();
    const hasFeature = (features || []).some((f) => f.key === featureKey);
    if (!hasFeature) {
      await client.features.create({
        key: featureKey,
        name: "Network spend",
        meterSlug: "network_fee_usd_micros",
      });
      console.log(`[openmeter] created feature ${featureKey}`);
    }
  } catch (err) {
    console.warn("[openmeter] feature bootstrap skipped:", err);
  }
}

export async function bootstrapOpenMeter(opts: {
  baseUrl: string;
  apiKey?: string;
  trialFeatureKey?: string;
}): Promise<void> {
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
