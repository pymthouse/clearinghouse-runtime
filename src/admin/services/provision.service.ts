import { buildCustomerKey } from "@pymthouse/konnect-metering";
import type { OpenMeterAdmin } from "../port.js";
import type { ProvisionCustomerInput, ProvisionResult } from "../types.js";

const ACTIVE_SUBSCRIPTION_STATUSES = new Set([
  "active",
  "scheduled",
  "pending",
]);

async function ensureCustomer(
  admin: OpenMeterAdmin,
  input: {
    key: string;
    name: string;
    subjectKeys: string[];
  },
): Promise<{ customerId: string; created: boolean }> {
  const existing = await admin.listCustomers({ key: input.key });
  const match = existing.find((c) => c.key === input.key);
  if (match?.id) {
    return { customerId: match.id, created: false };
  }

  const created = await admin.createCustomer(input);
  return { customerId: created.id, created: true };
}

async function ensureSubscription(
  admin: OpenMeterAdmin,
  input: {
    customerId: string;
    planKey: string;
  },
): Promise<{ subscriptionId: string; status: string; created: boolean }> {
  const existing = await admin.listCustomerSubscriptions(input.customerId);
  const match = existing.find(
    (sub) =>
      sub.planKey === input.planKey &&
      ACTIVE_SUBSCRIPTION_STATUSES.has(sub.status),
  );
  if (match?.id) {
    return {
      subscriptionId: match.id,
      status: match.status,
      created: false,
    };
  }

  const created = await admin.createSubscription(input);
  return {
    subscriptionId: created.id,
    status: created.status,
    created: true,
  };
}

export async function provisionCustomer(
  admin: OpenMeterAdmin,
  input: ProvisionCustomerInput,
): Promise<ProvisionResult> {
  const customerKey = buildCustomerKey(input.clientId, input.externalUserId);

  const customer = await ensureCustomer(admin, {
    key: customerKey,
    name: customerKey,
    subjectKeys: [customerKey],
  });

  const subscription = await ensureSubscription(admin, {
    customerId: customer.customerId,
    planKey: input.planKey,
  });

  return {
    customerKey,
    customerId: customer.customerId,
    subscriptionId: subscription.subscriptionId,
    planKey: input.planKey,
    status: subscription.status,
    created: {
      customer: customer.created,
      subscription: subscription.created,
    },
  };
}
