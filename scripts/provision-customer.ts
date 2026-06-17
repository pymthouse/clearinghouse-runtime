#!/usr/bin/env node
import {
  createAdmin,
  provisionCustomer,
} from "../src/admin/index.js";
import { AdminError } from "../src/admin/errors.js";
import { defaultPlanKey } from "../src/lib/pricing.js";

function parseArgs(argv: string[]): {
  clientId: string;
  externalUserId: string;
} {
  let clientId = process.env.AUTH0_PUBLIC_CLIENT_ID?.trim() || "";
  let externalUserId = "";

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--client-id" && argv[i + 1]) {
      clientId = argv[++i].trim();
      continue;
    }
    if (arg === "--external-user-id" && argv[i + 1]) {
      externalUserId = argv[++i].trim();
      continue;
    }
    if (arg === "--help" || arg === "-h") {
      console.log(`Usage: pnpm provision:customer -- [options]

Options:
  --client-id <id>           Auth0 SPA client id (azp); default AUTH0_PUBLIC_CLIENT_ID
  --external-user-id <sub>   Auth0 user subject (required)

Environment:
  OPENMETER_URL              Konnect or self-hosted OpenMeter base URL
  OPENMETER_API_KEY          API key (kpat_… for Konnect)
  OPENMETER_DEFAULT_PLAN_KEY Optional plan key override
`);
      process.exit(0);
    }
  }

  if (!clientId) {
    throw new Error(
      "client-id is required (--client-id or AUTH0_PUBLIC_CLIENT_ID)",
    );
  }
  if (!externalUserId) {
    throw new Error("external-user-id is required (--external-user-id)");
  }

  return { clientId, externalUserId };
}

async function main() {
  const baseUrl = process.env.OPENMETER_URL?.trim();
  const apiKey = process.env.OPENMETER_API_KEY?.trim();

  if (!baseUrl) {
    throw new Error("OPENMETER_URL is required");
  }
  if (!apiKey) {
    throw new Error("OPENMETER_API_KEY is required");
  }

  const { clientId, externalUserId } = parseArgs(process.argv.slice(2));
  const planKey = defaultPlanKey();

  console.log(`[provision] base: ${baseUrl}`);
  console.log(`[provision] plan: ${planKey}`);

  const admin = createAdmin({ baseUrl, apiKey });
  const result = await provisionCustomer(admin, {
    clientId,
    externalUserId,
    planKey,
  });

  console.log(`[provision] customer key: ${result.customerKey}`);

  if (result.created.customer) {
    console.log(
      `[provision] created customer: ${result.customerKey} (${result.customerId})`,
    );
  } else {
    console.log(
      `[provision] customer exists: ${result.customerKey} (${result.customerId})`,
    );
  }

  if (result.created.subscription) {
    console.log(
      `[provision] created subscription: ${result.subscriptionId} (plan ${planKey})`,
    );
  } else {
    console.log(
      `[provision] subscription exists: ${result.subscriptionId} (plan ${planKey})`,
    );
  }

  console.log(JSON.stringify(result, null, 2));
}

main().catch((err) => {
  const message =
    err instanceof AdminError
      ? `${err.name}: ${err.message}`
      : err instanceof Error
        ? err.message
        : String(err);
  console.error("[provision] failed:", message);
  process.exit(1);
});
