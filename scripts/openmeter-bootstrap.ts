#!/usr/bin/env node
import {
  bootstrapCatalog,
  createAdmin,
} from "../src/admin/index.js";
import { AdminError } from "../src/admin/errors.js";
import { isKonnectMeteringUrl } from "@pymthouse/konnect-metering";

const baseUrl = process.env.OPENMETER_URL;
const apiKey = process.env.OPENMETER_API_KEY;
const trialFeatureKey = process.env.OPENMETER_TRIAL_FEATURE_KEY;

if (!baseUrl) {
  console.error(
    "[openmeter-bootstrap] OPENMETER_URL is required\n" +
      "  Konnect: https://us.api.konghq.com/v3/openmeter\n" +
      "  Self-hosted: https://your-openmeter-host",
  );
  process.exit(1);
}

function logBootstrapResult(
  result: Awaited<ReturnType<typeof bootstrapCatalog>>,
  logPrefix: string,
): void {
  console.log(
    `[bootstrap] pricing: plan=${result.pricing.planKey} billable_feature=${result.pricing.billableFeatureKey} trial_micros=${result.pricing.trialIncludedUsdMicros}`,
  );

  for (const meter of result.meters) {
    if (meter.created) {
      console.log(`${logPrefix} created meter ${meter.resource.key}`);
    } else if (meter.warnings?.length) {
      for (const warning of meter.warnings) {
        console.warn(`${logPrefix} ${warning}`);
      }
    } else {
      console.log(`${logPrefix} meter exists: ${meter.resource.key}`);
    }
  }

  for (const feature of result.features) {
    if (feature.created) {
      console.log(`${logPrefix} created feature ${feature.resource.key}`);
    } else if (feature.warnings?.length) {
      for (const warning of feature.warnings) {
        console.warn(`${logPrefix} ${warning}`);
      }
    } else {
      console.log(`${logPrefix} feature exists: ${feature.resource.key}`);
    }
  }

  if (result.plan) {
    const included = result.pricing.trialIncludedUsdMicros;
    console.log(
      `${logPrefix} plan ready: ${result.plan.resource.key} (${result.plan.resource.id}) with ${included} included billable micros`,
    );
  } else if (result.planSkipped) {
    console.warn(`${logPrefix} plan bootstrap skipped: ${result.planSkipped.reason}`);
    console.log(
      `${logPrefix} Target plan key: ${result.pricing.planKey} (create manually on self-hosted if needed)`,
    );
  }

  console.log(`${logPrefix} Per-customer subscriptions are created at provision time:`);
  console.log(`  - default plan: ${result.provisionHint.defaultPlanKey}`);
  console.log(`  - ${result.provisionHint.provisionCommand}`);
  console.log(
    `  - Customer key format: ${result.provisionHint.customerKeyFormat} (Auth0 azp:sub)`,
  );
}

try {
  const admin = createAdmin({ baseUrl, apiKey });
  const backendLabel = isKonnectMeteringUrl(baseUrl, apiKey) ? "konnect" : "openmeter";
  console.log(`[bootstrap] provisioning ${backendLabel} at ${baseUrl}`);
  const result = await bootstrapCatalog(admin, { trialFeatureKey });
  logBootstrapResult(result, `[${backendLabel}]`);
} catch (err) {
  const message =
    err instanceof AdminError
      ? `${err.name}: ${err.message}`
      : err instanceof Error
        ? err.message
        : String(err);
  console.error("[openmeter-bootstrap] failed:", message);
  process.exit(1);
}
