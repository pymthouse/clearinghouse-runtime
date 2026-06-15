#!/usr/bin/env node
import { bootstrapOpenMeter } from "./lib/openmeter.js";

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

try {
  await bootstrapOpenMeter({ baseUrl, apiKey, trialFeatureKey });
} catch (err) {
  console.error("[openmeter-bootstrap] failed:", err);
  process.exit(1);
}
