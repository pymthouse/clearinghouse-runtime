import { OpenMeter } from "@openmeter/sdk";
import { BackendUnreachableError, ResourceNotFoundError } from "../errors.js";
import type { OpenMeterAdmin } from "../port.js";
import type {
  AdminCapabilities,
  Customer,
  CustomerInput,
  CustomerListQuery,
  Feature,
  FeatureInput,
  Meter,
  MeterInput,
  Plan,
  PlanInput,
  Subscription,
  SubscriptionInput,
} from "../types.js";

const ACTIVE_SUBSCRIPTION_STATUSES = new Set([
  "active",
  "scheduled",
  "pending",
]);

function aggregationToSdk(
  aggregation: MeterInput["aggregation"],
): "SUM" | "COUNT" | "AVG" | "MIN" | "MAX" {
  return aggregation === "count" ? "COUNT" : "SUM";
}

function mapMeterFromSdk(m: {
  id?: string;
  slug?: string;
  description?: string;
  groupBy?: Record<string, string>;
}): Meter {
  const key = m.slug ?? "";
  return {
    id: m.id ?? key,
    key,
    name: m.description,
    dimensions: m.groupBy,
  };
}

function mapFeatureFromSdk(m: {
  id?: string;
  key?: string;
  name?: string;
  meterSlug?: string;
}): Feature {
  return {
    id: m.id ?? m.key ?? "",
    key: m.key ?? "",
    name: m.name,
    meterId: m.meterSlug,
  };
}

function mapPlanFromSdk(m: {
  id?: string;
  key?: string;
  name?: string;
  status?: string;
}): Plan {
  return {
    id: m.id ?? "",
    key: m.key ?? "",
    name: m.name,
    status: m.status,
  };
}

function mapCustomerFromSdk(m: {
  id?: string;
  key?: string;
  name?: string;
  usageAttribution?: { subjectKeys?: string[] };
}): Customer {
  return {
    id: m.id ?? "",
    key: m.key ?? m.id ?? "",
    name: m.name,
    subjectKeys: m.usageAttribution?.subjectKeys,
  };
}

function mapSubscriptionFromSdk(m: {
  id?: string;
  status?: string;
  customerId?: string;
  plan?: { key?: string };
}): Subscription {
  return {
    id: m.id ?? "",
    status: m.status ?? "unknown",
    planKey: m.plan?.key,
    customerId: m.customerId,
  };
}

export class SelfHostedAdmin implements OpenMeterAdmin {
  readonly capabilities: AdminCapabilities = {
    plans: "full",
    subscriptions: "full",
  };

  private readonly baseUrl: string;
  private readonly client: OpenMeter;

  constructor(opts: { baseUrl: string; apiKey?: string }) {
    this.baseUrl = opts.baseUrl.replace(/\/$/, "");
    this.client = opts.apiKey?.trim()
      ? new OpenMeter({ baseUrl: this.baseUrl, apiKey: opts.apiKey.trim() })
      : new OpenMeter({ baseUrl: this.baseUrl });
  }

  async waitForHealthy(attempts = 30, delayMs = 2000): Promise<void> {
    for (let i = 0; i < attempts; i++) {
      try {
        const resp = await fetch(`${this.baseUrl}/api/v1/debug/metrics`);
        if (resp.ok) {
          return;
        }
      } catch {
        /* retry */
      }
      await new Promise((r) => setTimeout(r, delayMs));
    }
    throw new BackendUnreachableError(`OpenMeter not healthy at ${this.baseUrl}`);
  }

  async listMeters(): Promise<Meter[]> {
    const meters = await this.client.meters.list();
    const rows = Array.isArray(meters) ? meters : [];
    return rows.map(mapMeterFromSdk);
  }

  async createMeter(input: MeterInput): Promise<Meter> {
    const created = await this.client.meters.create({
      slug: input.key,
      description: input.description ?? input.name,
      eventType: input.eventType,
      aggregation: aggregationToSdk(input.aggregation),
      valueProperty: input.valueProperty,
      groupBy: input.dimensions,
    });
    return mapMeterFromSdk(created ?? { slug: input.key });
  }

  async listFeatures(): Promise<Feature[]> {
    const features = await this.client.features.list();
    const rows = Array.isArray(features) ? features : [];
    return rows.map(mapFeatureFromSdk);
  }

  async createFeature(input: FeatureInput): Promise<Feature> {
    const meters = await this.listMeters();
    const meter = meters.find(
      (m) => m.id === input.meterId || m.key === input.meterId,
    );
    const meterSlug = meter?.key ?? input.meterId;

    const created = await this.client.features.create({
      key: input.key,
      name: input.name,
      meterSlug,
    });
    return mapFeatureFromSdk(created ?? { key: input.key, name: input.name });
  }

  async listPlans(): Promise<Plan[]> {
    const result = await this.client.plans.list();
    const items = result?.items ?? [];
    return items.map(mapPlanFromSdk);
  }

  async ensurePlan(input: PlanInput): Promise<Plan> {
    const existing = (await this.listPlans()).find((p) => p.key === input.key);
    if (existing) {
      return existing;
    }

    const created = await this.client.plans.create({
      key: input.key,
      name: input.name,
      currency: (input.currency ?? "USD") as "USD",
      billingCadence: input.billingCadence ?? "P1M",
      phases: [
        {
          key: "default",
          name: "Default",
          duration: null,
          rateCards: [
            {
              type: "usage_based",
              key: input.featureKey,
              name: input.featureName,
              featureKey: input.featureKey,
              billingCadence: input.billingCadence ?? "P1M",
              price: {
                type: "unit",
                amount: input.unitAmount,
              },
            },
          ],
        },
      ],
    });

    if (!created?.id) {
      throw new ResourceNotFoundError("plan", input.key);
    }

    return mapPlanFromSdk(created);
  }

  async listCustomers(query?: CustomerListQuery): Promise<Customer[]> {
    const result = await this.client.customers.list({
      page: query?.page ?? 1,
      pageSize: query?.pageSize ?? 100,
      ...(query?.key ? { key: query.key } : {}),
    });

    const items = result?.items ?? (Array.isArray(result) ? result : []);
    return (items as Array<Parameters<typeof mapCustomerFromSdk>[0]>).map(
      mapCustomerFromSdk,
    );
  }

  async createCustomer(input: CustomerInput): Promise<Customer> {
    const created = await this.client.customers.create({
      key: input.key,
      name: input.name,
      usageAttribution: { subjectKeys: input.subjectKeys },
    });

    if (!created?.id) {
      throw new ResourceNotFoundError("customer", input.key);
    }

    return mapCustomerFromSdk(created);
  }

  async listCustomerSubscriptions(customerId: string): Promise<Subscription[]> {
    const result = await this.client.customers.listSubscriptions(customerId);
    const items = result?.items ?? (Array.isArray(result) ? result : []);
    return (items as Array<Parameters<typeof mapSubscriptionFromSdk>[0]>).map(
      (s) => mapSubscriptionFromSdk({ ...s, customerId }),
    );
  }

  async createSubscription(input: SubscriptionInput): Promise<Subscription> {
    const existing = await this.listCustomerSubscriptions(input.customerId);
    const match = existing.find(
      (sub) =>
        sub.planKey === input.planKey &&
        ACTIVE_SUBSCRIPTION_STATUSES.has(sub.status),
    );
    if (match) {
      return match;
    }

    const created = await this.client.subscriptions.create({
      customerId: input.customerId,
      plan: { key: input.planKey },
    });

    if (!created?.id) {
      throw new ResourceNotFoundError("subscription", input.planKey);
    }

    return mapSubscriptionFromSdk({
      id: created.id,
      status: created.status,
      customerId: input.customerId,
      plan: created.plan,
    });
  }
}
