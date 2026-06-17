import {
  billableFeatureKey,
  defaultPlanKey,
  defaultTrialIncludedUsdMicros,
  type PricingConfig,
  unitPriceUsdPerBillableMicro,
} from "./pricing.js";

type KonnectPlan = {
  id: string;
  key: string;
};

type KonnectFeature = {
  id: string;
  key: string;
};

type PageResponse<T> = {
  data?: T[];
};

export async function konnectFetch<T>(
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

async function findKonnectFeatureIdByKey(
  baseUrl: string,
  apiKey: string,
  featureKey: string,
): Promise<string | null> {
  const features = await konnectFetch<PageResponse<KonnectFeature>>(
    baseUrl,
    apiKey,
    "/features",
  );
  return (features.data ?? []).find((feature) => feature.key === featureKey)?.id ?? null;
}

function buildKonnectUsageRateCard(input: {
  key: string;
  name: string;
  featureId: string;
  unitAmount: string;
  includedMicros: number;
}): Record<string, unknown> {
  const card: Record<string, unknown> = {
    key: input.key,
    name: input.name,
    feature: { id: input.featureId },
    billing_cadence: "P1M",
    price: {
      type: "unit",
      amount: input.unitAmount,
    },
  };

  if (input.includedMicros > 0) {
    card.discounts = {
      usage: String(input.includedMicros),
    };
  }

  return card;
}

export async function ensureKonnectDefaultPayPerUsePlan(input: {
  baseUrl: string;
  apiKey: string;
  billableMeterId: string;
  pricing?: Pick<
    PricingConfig,
    | "defaultPlanKey"
    | "billableFeatureKey"
    | "unitPriceUsdPerBillableMicro"
    | "defaultTrialIncludedUsdMicros"
  >;
}): Promise<{ planId: string; planKey: string; created: boolean }> {
  const planKey = input.pricing?.defaultPlanKey?.trim() || defaultPlanKey();
  const featureKey =
    input.pricing?.billableFeatureKey?.trim() || billableFeatureKey();
  const unitAmount =
    input.pricing?.unitPriceUsdPerBillableMicro?.trim() ||
    unitPriceUsdPerBillableMicro();
  const includedRaw =
    input.pricing?.defaultTrialIncludedUsdMicros?.trim() ||
    defaultTrialIncludedUsdMicros();
  const includedMicros = Math.max(0, Math.floor(Number(includedRaw) || 0));

  let featureId = await findKonnectFeatureIdByKey(
    input.baseUrl,
    input.apiKey,
    featureKey,
  );
  if (!featureId) {
    const created = await konnectFetch<KonnectFeature>(
      input.baseUrl,
      input.apiKey,
      "/features",
      {
        method: "POST",
        body: JSON.stringify({
          key: featureKey,
          name: "Billable spend",
          meter: { id: input.billableMeterId },
        }),
      },
    );
    featureId = created.id;
    console.log(`[konnect] created feature ${featureKey}`);
  }

  const listed = await konnectFetch<PageResponse<KonnectPlan>>(
    input.baseUrl,
    input.apiKey,
    "/plans",
  );
  const existing = (listed.data ?? []).find((plan) => plan.key === planKey);
  if (existing?.id) {
    console.log(`[konnect] plan exists: ${planKey} (${existing.id})`);
    return { planId: existing.id, planKey, created: false };
  }

  const rateCard = buildKonnectUsageRateCard({
    key: featureKey,
    name: "Billable usage",
    featureId,
    unitAmount,
    includedMicros,
  });

  const createdPlan = await konnectFetch<KonnectPlan>(
    input.baseUrl,
    input.apiKey,
    "/plans",
    {
      method: "POST",
      body: JSON.stringify({
        key: planKey,
        name: "Clearinghouse Default Pay-Per-Use",
        currency: "USD",
        billing_cadence: "P1M",
        phases: [
          {
            key: "default",
            name: "Default",
            rate_cards: [rateCard],
          },
        ],
      }),
    },
  );

  console.log(
    `[konnect] created plan ${planKey} (${createdPlan.id}) with ${includedMicros} included billable micros`,
  );

  return { planId: createdPlan.id, planKey, created: true };
}
