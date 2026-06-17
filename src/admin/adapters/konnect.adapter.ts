import {
  createKonnectClient,
  type KonnectClient,
} from "@pymthouse/konnect-metering";
import type { components } from "@pymthouse/konnect-metering";
import { BackendUnreachableError } from "../errors.js";
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

type ApiMeter = components["schemas"]["Meter"];
type ApiFeature = components["schemas"]["Feature"];
type ApiPlan = components["schemas"]["BillingPlan"];
type ApiCustomer = components["schemas"]["BillingCustomer"];
type ApiSubscription = components["schemas"]["BillingSubscription"];

function mapMeter(m: ApiMeter): Meter {
  return {
    id: m.id,
    key: m.key,
    name: m.name,
    dimensions: m.dimensions,
  };
}

function mapFeature(m: ApiFeature): Feature {
  return {
    id: m.id,
    key: m.key,
    name: m.name,
    meterId: m.meter?.id,
  };
}

function mapPlan(m: ApiPlan): Plan {
  return {
    id: m.id,
    key: m.key,
    name: m.name,
    status: m.status,
  };
}

function mapCustomer(m: ApiCustomer): Customer {
  return {
    id: m.id,
    key: m.key ?? m.id,
    name: m.name,
    subjectKeys: m.usage_attribution?.subject_keys,
  };
}

function mapSubscription(
  m: ApiSubscription,
  planKey?: string,
  customerId?: string,
): Subscription {
  return {
    id: m.id,
    status: m.status,
    planKey,
    customerId: customerId ?? m.customer_id,
  };
}

function buildUsageRateCard(input: {
  key: string;
  name: string;
  featureId: string;
  unitAmount: string;
  includedMicros: number;
}): components["schemas"]["BillingRateCard"] {
  const card: components["schemas"]["BillingRateCard"] = {
    key: input.key,
    name: input.name,
    feature: { id: input.featureId },
    billing_cadence: "P1M",
    payment_term: "in_arrears",
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

export class KonnectAdmin implements OpenMeterAdmin {
  readonly capabilities: AdminCapabilities = {
    plans: "full",
    subscriptions: "full",
  };

  private readonly client: KonnectClient;

  constructor(opts: { baseUrl: string; apiKey: string }) {
    this.client = createKonnectClient(opts);
  }

  async waitForHealthy(): Promise<void> {
    try {
      await this.client.waitForHealthy();
    } catch (err) {
      throw new BackendUnreachableError(
        err instanceof Error ? err.message : String(err),
      );
    }
  }

  async listMeters(): Promise<Meter[]> {
    const { data } = await this.client.api.GET("/openmeter/meters");
    return (data?.data ?? []).map(mapMeter);
  }

  async createMeter(input: MeterInput): Promise<Meter> {
    const { data } = await this.client.api.POST("/openmeter/meters", {
      body: {
        key: input.key,
        name: input.name,
        description: input.description,
        event_type: input.eventType,
        aggregation: input.aggregation,
        value_property: input.valueProperty,
        dimensions: input.dimensions,
      },
    });
    if (!data) {
      throw new Error(`Failed to create meter ${input.key}`);
    }
    return mapMeter(data);
  }

  async listFeatures(): Promise<Feature[]> {
    const { data } = await this.client.api.GET("/openmeter/features");
    return (data?.data ?? []).map(mapFeature);
  }

  async createFeature(input: FeatureInput): Promise<Feature> {
    const { data } = await this.client.api.POST("/openmeter/features", {
      body: {
        key: input.key,
        name: input.name,
        meter: { id: input.meterId },
      },
    });
    if (!data) {
      throw new Error(`Failed to create feature ${input.key}`);
    }
    return mapFeature(data);
  }

  async listPlans(): Promise<Plan[]> {
    const { data } = await this.client.api.GET("/openmeter/plans");
    return (data?.data ?? []).map(mapPlan);
  }

  async ensurePlan(input: PlanInput): Promise<Plan> {
    const includedMicros = Math.max(0, Math.floor(input.includedMicros));

    const features = await this.listFeatures();
    let featureId = features.find((f) => f.key === input.featureKey)?.id;

    if (!featureId) {
      const created = await this.createFeature({
        key: input.featureKey,
        name: input.featureName,
        meterId: input.billableMeterId,
      });
      featureId = created.id;
    }

    const rateCard = buildUsageRateCard({
      key: input.featureKey,
      name: "Billable usage",
      featureId,
      unitAmount: input.unitAmount,
      includedMicros,
    });

    const { data: createdPlan } = await this.client.api.POST("/openmeter/plans", {
      body: {
        key: input.key,
        name: input.name,
        currency: input.currency ?? "USD",
        billing_cadence: input.billingCadence ?? "P1M",
        pro_rating_enabled: true,
        phases: [
          {
            key: "default",
            name: "Default",
            rate_cards: [rateCard],
          },
        ],
      },
    });

    if (!createdPlan) {
      throw new Error(`Failed to create plan ${input.key}`);
    }

    if (createdPlan.status === "draft") {
      await this.client.api.POST("/openmeter/plans/{planId}/publish", {
        params: { path: { planId: createdPlan.id } },
      });
    }

    return {
      id: createdPlan.id,
      key: input.key,
      name: input.name,
    };
  }

  async listCustomers(query?: CustomerListQuery): Promise<Customer[]> {
    if (query?.key) {
      const { data } = await this.client.api.GET("/openmeter/customers", {
        params: {
          query: {
            filter: { key: query.key },
            page: { number: 1, size: 100 },
          },
        },
      });
      const rows = data?.data ?? [];
      return rows.filter((row) => row.key === query.key).map(mapCustomer);
    }

    const { data } = await this.client.api.GET("/openmeter/customers", {
      params: {
        query: {
          page: {
            number: query?.page ?? 1,
            size: query?.pageSize ?? 100,
          },
        },
      },
    });
    return (data?.data ?? []).map(mapCustomer);
  }

  async createCustomer(input: CustomerInput): Promise<Customer> {
    const { data } = await this.client.api.POST("/openmeter/customers", {
      body: {
        key: input.key,
        name: input.name,
        usage_attribution: { subject_keys: input.subjectKeys },
      },
    });
    if (!data) {
      throw new Error(`Failed to create customer ${input.key}`);
    }
    return mapCustomer(data);
  }

  async listCustomerSubscriptions(customerId: string): Promise<Subscription[]> {
    const { data } = await this.client.api.GET("/openmeter/subscriptions", {
      params: {
        query: {
          filter: { customer_id: customerId },
        },
      },
    });
    const subs = data?.data ?? [];
    const plans = subs.some((sub) => sub.plan_id)
      ? await this.listPlans()
      : [];
    const planKeyById = new Map(plans.map((plan) => [plan.id, plan.key]));

    return subs.map((sub) =>
      mapSubscription(
        sub,
        sub.plan_id ? planKeyById.get(sub.plan_id) : undefined,
        customerId,
      ),
    );
  }

  async createSubscription(input: SubscriptionInput): Promise<Subscription> {
    const { data } = await this.client.api.POST("/openmeter/subscriptions", {
      body: {
        customer: { id: input.customerId },
        plan: { key: input.planKey },
      },
    });
    if (!data) {
      throw new Error(
        `Failed to create subscription for plan ${input.planKey}`,
      );
    }
    return mapSubscription(data, input.planKey, input.customerId);
  }
}
