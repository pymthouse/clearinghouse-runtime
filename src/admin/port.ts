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
} from "./types.js";

export interface OpenMeterAdmin {
  readonly capabilities: AdminCapabilities;
  waitForHealthy(): Promise<void>;
  listMeters(): Promise<Meter[]>;
  createMeter(input: MeterInput): Promise<Meter>;
  listFeatures(): Promise<Feature[]>;
  createFeature(input: FeatureInput): Promise<Feature>;
  listPlans(): Promise<Plan[]>;
  ensurePlan(input: PlanInput): Promise<Plan>;
  listCustomers(query?: CustomerListQuery): Promise<Customer[]>;
  createCustomer(input: CustomerInput): Promise<Customer>;
  listCustomerSubscriptions(customerId: string): Promise<Subscription[]>;
  createSubscription(input: SubscriptionInput): Promise<Subscription>;
}
