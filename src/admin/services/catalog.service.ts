import {
  BILLABLE_USD_MICROS_METER,
  DEFAULT_BILLABLE_FEATURE_KEY,
  DEFAULT_TRIAL_FEATURE_KEY,
  KONNECT_METER_DEFINITIONS,
  NETWORK_FEE_USD_MICROS_METER,
} from "../../lib/meters.js";
import {
  billableFeatureKey,
  defaultPlanKey,
  defaultTrialIncludedUsdMicros,
  unitPriceUsdPerBillableMicro,
} from "../../lib/pricing.js";
import type { OpenMeterAdmin } from "../port.js";
import type {
  BootstrapResult,
  EnsureResult,
  Feature,
  FeatureInput,
  Meter,
  MeterInput,
  Plan,
} from "../types.js";

function konnectMeterToInput(
  m: (typeof KONNECT_METER_DEFINITIONS)[number],
): MeterInput {
  return {
    key: m.key,
    name: m.name,
    description: m.description,
    eventType: m.event_type,
    aggregation: m.aggregation,
    valueProperty: m.value_property,
    dimensions: m.dimensions,
  };
}

const BOOTSTRAP_METER_INPUTS = KONNECT_METER_DEFINITIONS.map(konnectMeterToInput);

export async function ensureMeter(
  admin: OpenMeterAdmin,
  input: MeterInput,
): Promise<EnsureResult<Meter>> {
  const existing = await admin.listMeters();
  const match = existing.find((m) => m.key === input.key);
  if (match) {
    const dimensions = match.dimensions ?? {};
    const hasPipelineDimensions =
      "pipeline" in dimensions && "model_id" in dimensions;
    const warnings: string[] = [];
    if (!hasPipelineDimensions) {
      warnings.push(
        `meter ${input.key} missing pipeline/model_id dimensions — recreate manually if needed`,
      );
    }
    return { resource: match, created: false, warnings };
  }

  const created = await admin.createMeter(input);
  return { resource: created, created: true };
}

export async function ensureFeature(
  admin: OpenMeterAdmin,
  input: FeatureInput,
): Promise<EnsureResult<Feature>> {
  const features = await admin.listFeatures();
  const match = features.find((f) => f.key === input.key);
  if (match) {
    return { resource: match, created: false };
  }

  try {
    const created = await admin.createFeature(input);
    return { resource: created, created: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return {
      resource: {
        id: "",
        key: input.key,
        name: input.name,
        meterId: input.meterId,
      },
      created: false,
      warnings: [`feature bootstrap skipped for ${input.key}: ${message}`],
    };
  }
}

export async function ensureDefaultPlan(
  admin: OpenMeterAdmin,
  input: {
    key: string;
    name: string;
    featureKey: string;
    featureName: string;
    billableMeterId: string;
    unitAmount: string;
    includedMicros: number;
    currency?: string;
    billingCadence?: string;
  },
): Promise<EnsureResult<Plan>> {
  const existing = (await admin.listPlans()).find((p) => p.key === input.key);
  if (existing) {
    return { resource: existing, created: false };
  }

  const created = await admin.ensurePlan({
    key: input.key,
    name: input.name,
    featureKey: input.featureKey,
    featureName: input.featureName,
    billableMeterId: input.billableMeterId,
    unitAmount: input.unitAmount,
    includedMicros: input.includedMicros,
    currency: input.currency,
    billingCadence: input.billingCadence,
  });
  return { resource: created, created: true };
}

export type BootstrapCatalogOptions = {
  trialFeatureKey?: string;
};

export async function bootstrapCatalog(
  admin: OpenMeterAdmin,
  opts: BootstrapCatalogOptions = {},
): Promise<BootstrapResult> {
  const planKey = defaultPlanKey();

  await admin.waitForHealthy();

  const meters: EnsureResult<Meter>[] = [];
  for (const meterInput of BOOTSTRAP_METER_INPUTS) {
    meters.push(await ensureMeter(admin, meterInput));
  }

  const meterList = await admin.listMeters();
  const networkFeeMeter = meterList.find(
    (m) => m.key === NETWORK_FEE_USD_MICROS_METER,
  );
  if (!networkFeeMeter) {
    throw new Error(`Meter missing after bootstrap: ${NETWORK_FEE_USD_MICROS_METER}`);
  }

  const billableMeter = meterList.find(
    (m) => m.key === BILLABLE_USD_MICROS_METER,
  );
  if (!billableMeter) {
    throw new Error(`Meter missing after bootstrap: ${BILLABLE_USD_MICROS_METER}`);
  }

  const networkFeatureKey =
    opts.trialFeatureKey?.trim() || DEFAULT_TRIAL_FEATURE_KEY;
  const billableFeatureKeyResolved =
    billableFeatureKey().trim() || DEFAULT_BILLABLE_FEATURE_KEY;

  const features: EnsureResult<Feature>[] = [];
  features.push(
    await ensureFeature(admin, {
      key: networkFeatureKey,
      name: "Network spend",
      meterId: networkFeeMeter.id,
    }),
  );
  features.push(
    await ensureFeature(admin, {
      key: billableFeatureKeyResolved,
      name: "Billable spend",
      meterId: billableMeter.id,
    }),
  );

  const includedMicros = Math.max(
    0,
    Math.floor(Number(defaultTrialIncludedUsdMicros()) || 0),
  );

  const result: BootstrapResult = {
    pricing: {
      planKey,
      billableFeatureKey: billableFeatureKeyResolved,
      trialIncludedUsdMicros: defaultTrialIncludedUsdMicros(),
    },
    meters,
    features,
    provisionHint: {
      defaultPlanKey: planKey,
      customerKeyFormat: "{client_id}:{external_user_id}",
      provisionCommand:
        "pnpm provision:customer -- --client-id <AUTH0_PUBLIC_CLIENT_ID> --external-user-id <auth0|sub>",
    },
  };

  if (admin.capabilities.plans === "none") {
    result.planSkipped = {
      reason: "plan bootstrap not supported on this backend",
    };
    return result;
  }

  try {
    result.plan = await ensureDefaultPlan(admin, {
      key: planKey,
      name: "Clearinghouse Default Pay-Per-Use",
      featureKey: billableFeatureKeyResolved,
      featureName: "Billable spend",
      billableMeterId: billableMeter.id,
      unitAmount: unitPriceUsdPerBillableMicro(),
      includedMicros,
      currency: "USD",
      billingCadence: "P1M",
    });
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    result.planSkipped = { reason: message };
  }

  return result;
}
