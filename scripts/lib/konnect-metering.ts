import {
  DEFAULT_TRIAL_FEATURE_KEY,
  KONNECT_METER_DEFINITIONS,
  NETWORK_FEE_USD_MICROS_METER,
} from "./meters.js";

export const DEFAULT_KONNECT_METERING_URL =
  "https://us.api.konghq.com/v3/openmeter";

type KonnectMeter = {
  id: string;
  key: string;
  dimensions?: Record<string, string>;
};

type KonnectFeature = {
  id: string;
  key: string;
};

type PageResponse<T> = {
  data?: T[];
};

export function isKonnectApiKey(apiKey: string | undefined): boolean {
  const key = apiKey?.trim() ?? "";
  return key.startsWith("kpat_") || key.startsWith("spat_");
}

export function isKonnectMeteringUrl(
  url: string,
  apiKey?: string,
): boolean {
  if (/konghq\.com/i.test(url)) {
    return true;
  }
  return isKonnectApiKey(apiKey);
}

export function normalizeKonnectMeteringUrl(url: string): string {
  let base = url.trim().replace(/\/$/, "");
  if (base.endsWith("/events")) {
    base = base.slice(0, -"/events".length);
  }
  if (!base.endsWith("/openmeter")) {
    if (/\/v\d+$/i.test(base)) {
      base = `${base}/openmeter`;
    }
  }
  return base;
}

async function konnectFetch<T>(
  baseUrl: string,
  apiKey: string,
  path: string,
  init?: RequestInit,
): Promise<T> {
  const resp = await fetch(`${baseUrl}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${apiKey}`,
      ...init?.headers,
    },
  });

  if (!resp.ok) {
    const body = await resp.text();
    throw new Error(
      `Konnect Metering API ${init?.method ?? "GET"} ${path} failed (${resp.status}): ${body}`,
    );
  }

  if (resp.status === 204) {
    return undefined as T;
  }

  return (await resp.json()) as T;
}

async function waitForKonnectHealthy(
  baseUrl: string,
  apiKey: string,
  attempts = 15,
): Promise<void> {
  for (let i = 0; i < attempts; i++) {
    try {
      const resp = await fetch(`${baseUrl}/meters`, {
        headers: { Authorization: `Bearer ${apiKey}` },
      });
      if (resp.ok) {
        return;
      }
    } catch {
      /* retry */
    }
    await new Promise((r) => setTimeout(r, 2000));
  }
  throw new Error(`Konnect Metering & Billing not reachable at ${baseUrl}`);
}

export async function bootstrapKonnectMetering(opts: {
  baseUrl: string;
  apiKey: string;
  trialFeatureKey?: string;
}): Promise<void> {
  const baseUrl = normalizeKonnectMeteringUrl(opts.baseUrl);
  const apiKey = opts.apiKey.trim();

  await waitForKonnectHealthy(baseUrl, apiKey);

  const listed = await konnectFetch<PageResponse<KonnectMeter>>(
    baseUrl,
    apiKey,
    "/meters",
  );
  const existingMeters = listed.data ?? [];

  for (const meter of KONNECT_METER_DEFINITIONS) {
    const existingMeter = existingMeters.find((m) => m.key === meter.key);
    if (existingMeter) {
      const dimensions = existingMeter.dimensions ?? {};
      const hasPipelineDimensions =
        "pipeline" in dimensions && "model_id" in dimensions;
      if (!hasPipelineDimensions) {
        console.warn(
          `[konnect] meter ${meter.key} missing pipeline/model_id dimensions — recreate manually if needed`,
        );
      }
      continue;
    }

    await konnectFetch<KonnectMeter>(baseUrl, apiKey, "/meters", {
      method: "POST",
      body: JSON.stringify(meter),
    });
    console.log(`[konnect] created meter ${meter.key}`);
  }

  const refreshed = await konnectFetch<PageResponse<KonnectMeter>>(
    baseUrl,
    apiKey,
    "/meters",
  );
  const networkFeeMeter = (refreshed.data ?? []).find(
    (m) => m.key === NETWORK_FEE_USD_MICROS_METER,
  );
  if (!networkFeeMeter) {
    console.warn(
      `[konnect] meter ${NETWORK_FEE_USD_MICROS_METER} not found — skipping feature bootstrap`,
    );
    return;
  }

  const featureKey = opts.trialFeatureKey?.trim() || DEFAULT_TRIAL_FEATURE_KEY;
  try {
    const features = await konnectFetch<PageResponse<KonnectFeature>>(
      baseUrl,
      apiKey,
      "/features",
    );
    const hasFeature = (features.data ?? []).some((f) => f.key === featureKey);
    if (!hasFeature) {
      await konnectFetch<KonnectFeature>(baseUrl, apiKey, "/features", {
        method: "POST",
        body: JSON.stringify({
          key: featureKey,
          name: "Network spend",
          meter: { id: networkFeeMeter.id },
        }),
      });
      console.log(`[konnect] created feature ${featureKey}`);
    }
  } catch (err) {
    console.warn("[konnect] feature bootstrap skipped:", err);
  }
}
