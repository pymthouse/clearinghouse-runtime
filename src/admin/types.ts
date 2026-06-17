export type MeterAggregation = "sum" | "count";

export type Meter = {
  id: string;
  key: string;
  name?: string;
  dimensions?: Record<string, string>;
};

export type MeterInput = {
  key: string;
  name: string;
  description?: string;
  eventType: string;
  aggregation: MeterAggregation;
  valueProperty?: string;
  dimensions: Record<string, string>;
};

export type Feature = {
  id: string;
  key: string;
  name?: string;
  meterId?: string;
};

export type FeatureInput = {
  key: string;
  name: string;
  meterId: string;
};

export type Plan = {
  id: string;
  key: string;
  name?: string;
  status?: string;
};

export type PlanInput = {
  key: string;
  name: string;
  featureKey: string;
  featureName: string;
  billableMeterId: string;
  unitAmount: string;
  includedMicros: number;
  currency?: string;
  billingCadence?: string;
};

export type Customer = {
  id: string;
  key: string;
  name?: string;
  subjectKeys?: string[];
};

export type CustomerInput = {
  key: string;
  name: string;
  subjectKeys: string[];
};

export type CustomerListQuery = {
  key?: string;
  page?: number;
  pageSize?: number;
};

export type Subscription = {
  id: string;
  status: string;
  planKey?: string;
  customerId?: string;
};

export type SubscriptionInput = {
  customerId: string;
  planKey: string;
};

export type AdminCapabilities = {
  plans: "full" | "none";
  subscriptions: "full";
};

export type EnsureResult<T> = {
  resource: T;
  created: boolean;
  warnings?: string[];
};

export type BootstrapResult = {
  pricing: {
    planKey: string;
    billableFeatureKey: string;
    trialIncludedUsdMicros: string;
  };
  meters: EnsureResult<Meter>[];
  features: EnsureResult<Feature>[];
  plan?: EnsureResult<Plan>;
  planSkipped?: { reason: string };
  provisionHint: {
    defaultPlanKey: string;
    customerKeyFormat: string;
    provisionCommand: string;
  };
};

export type ProvisionCustomerInput = {
  clientId: string;
  externalUserId: string;
  planKey: string;
};

export type ProvisionResult = {
  customerKey: string;
  customerId: string;
  subscriptionId: string;
  planKey: string;
  status: string;
  created: {
    customer: boolean;
    subscription: boolean;
  };
};
