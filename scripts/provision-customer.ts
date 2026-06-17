#!/usr/bin/env node
import {
  DEFAULT_KONNECT_METERING_URL,
  isKonnectMeteringUrl,
  normalizeKonnectMeteringUrl,
} from "./lib/konnect-metering.js";
import { konnectFetch } from "./lib/konnect-default-plan.js";
import { defaultPlanKey } from "./lib/pricing.js";

type KonnectCustomer = {
  id: string;
  key?: string;
};

type KonnectSubscription = {
  id: string;
  status: string;
  plan?: { key?: string; id?: string } | null;
};

type PageResponse<T> = {
  data?: T[];
  items?: T[];
};

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
  OPENMETER_URL              Konnect metering base URL
  OPENMETER_API_KEY          Konnect PAT (kpat_…)
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

function buildCustomerKey(clientId: string, externalUserId: string): string {
  return `${clientId}:${externalUserId}`;
}

async function findCustomerByKey(
  baseUrl: string,
  apiKey: string,
  customerKey: string,
): Promise<KonnectCustomer | null> {
  const listed = await konnectFetch<PageResponse<KonnectCustomer>>(
    baseUrl,
    apiKey,
    `/customers?key=${encodeURIComponent(customerKey)}&page=1&pageSize=100`,
  );
  const rows = listed.data ?? listed.items ?? [];
  return rows.find((row) => row.key === customerKey) ?? rows[0] ?? null;
}

async function ensureCustomer(
  baseUrl: string,
  apiKey: string,
  input: {
    clientId: string;
    externalUserId: string;
  },
): Promise<KonnectCustomer> {
  const customerKey = buildCustomerKey(input.clientId, input.externalUserId);
  const existing = await findCustomerByKey(baseUrl, apiKey, customerKey);
  if (existing?.id) {
    console.log(`[provision] customer exists: ${customerKey} (${existing.id})`);
    return existing;
  }

  const created = await konnectFetch<KonnectCustomer>(baseUrl, apiKey, "/customers", {
    method: "POST",
    body: JSON.stringify({
      key: customerKey,
      name: customerKey,
      usage_attribution: {
        subject_keys: [customerKey],
      },
    }),
  });

  if (!created?.id) {
    throw new Error(`Failed to create customer for key ${customerKey}`);
  }

  console.log(`[provision] created customer: ${customerKey} (${created.id})`);
  return created;
}

async function listCustomerSubscriptions(
  baseUrl: string,
  apiKey: string,
  customerId: string,
): Promise<KonnectSubscription[]> {
  const listed = await konnectFetch<PageResponse<KonnectSubscription>>(
    baseUrl,
    apiKey,
    `/customers/${encodeURIComponent(customerId)}/subscriptions`,
  );
  return listed.data ?? listed.items ?? [];
}

async function ensureSubscription(
  baseUrl: string,
  apiKey: string,
  input: {
    customerId: string;
    planKey: string;
  },
): Promise<KonnectSubscription> {
  const existing = await listCustomerSubscriptions(
    baseUrl,
    apiKey,
    input.customerId,
  );
  const match = existing.find(
    (sub) =>
      sub.plan?.key === input.planKey &&
      (sub.status === "active" ||
        sub.status === "scheduled" ||
        sub.status === "pending"),
  );
  if (match?.id) {
    console.log(
      `[provision] subscription exists: ${match.id} (plan ${input.planKey})`,
    );
    return match;
  }

  const created = await konnectFetch<KonnectSubscription>(
    baseUrl,
    apiKey,
    "/subscriptions",
    {
      method: "POST",
      body: JSON.stringify({
        customerId: input.customerId,
        plan: { key: input.planKey },
      }),
    },
  );

  if (!created?.id) {
    throw new Error(`Failed to create subscription for plan ${input.planKey}`);
  }

  console.log(
    `[provision] created subscription: ${created.id} (plan ${input.planKey})`,
  );
  return created;
}

async function main() {
  const baseUrlRaw = process.env.OPENMETER_URL?.trim();
  const apiKey = process.env.OPENMETER_API_KEY?.trim();

  if (!baseUrlRaw) {
    throw new Error("OPENMETER_URL is required");
  }
  if (!apiKey) {
    throw new Error("OPENMETER_API_KEY is required");
  }
  if (!isKonnectMeteringUrl(baseUrlRaw, apiKey)) {
    throw new Error(
      "provision:customer currently supports Konnect only — use kpat_ API key and Kong base URL",
    );
  }

  const { clientId, externalUserId } = parseArgs(process.argv.slice(2));
  const baseUrl = normalizeKonnectMeteringUrl(
    /konghq\.com/i.test(baseUrlRaw) ? baseUrlRaw : DEFAULT_KONNECT_METERING_URL,
  );
  const planKey = defaultPlanKey();
  const customerKey = buildCustomerKey(clientId, externalUserId);

  console.log(`[provision] Konnect base: ${baseUrl}`);
  console.log(`[provision] customer key: ${customerKey}`);
  console.log(`[provision] plan: ${planKey}`);

  const customer = await ensureCustomer(baseUrl, apiKey, {
    clientId,
    externalUserId,
  });
  const subscription = await ensureSubscription(baseUrl, apiKey, {
    customerId: customer.id,
    planKey,
  });

  console.log(
    JSON.stringify(
      {
        customerKey,
        customerId: customer.id,
        subscriptionId: subscription.id,
        planKey,
        status: subscription.status,
      },
      null,
      2,
    ),
  );
}

main().catch((err) => {
  console.error("[provision] failed:", err);
  process.exit(1);
});
