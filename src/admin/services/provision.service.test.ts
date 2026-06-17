import { describe, expect, it, vi } from "vitest";
import type { OpenMeterAdmin } from "../port.js";
import { provisionCustomer } from "./provision.service.js";

function mockAdmin(
  overrides: Partial<OpenMeterAdmin> = {},
): OpenMeterAdmin {
  return {
    capabilities: { plans: "full", subscriptions: "full" },
    waitForHealthy: vi.fn(),
    listMeters: vi.fn(),
    createMeter: vi.fn(),
    listFeatures: vi.fn(),
    createFeature: vi.fn(),
    listPlans: vi.fn(),
    ensurePlan: vi.fn(),
    listCustomers: vi.fn().mockResolvedValue([]),
    createCustomer: vi.fn().mockResolvedValue({
      id: "cust-new",
      key: "client:sub",
    }),
    listCustomerSubscriptions: vi.fn().mockResolvedValue([]),
    createSubscription: vi.fn().mockResolvedValue({
      id: "sub-new",
      status: "active",
      planKey: "default-plan",
    }),
    ...overrides,
  };
}

describe("provisionCustomer", () => {
  it("creates customer and subscription", async () => {
    const admin = mockAdmin();

    const result = await provisionCustomer(admin, {
      clientId: "client",
      externalUserId: "sub",
      planKey: "default-plan",
    });

    expect(result.customerKey).toBe("client:sub");
    expect(result.created.customer).toBe(true);
    expect(result.created.subscription).toBe(true);
    expect(admin.createCustomer).toHaveBeenCalledOnce();
    expect(admin.createSubscription).toHaveBeenCalledOnce();
  });

  it("reuses existing customer and subscription", async () => {
    const admin = mockAdmin({
      listCustomers: vi.fn().mockResolvedValue([
        { id: "cust-1", key: "client:sub" },
      ]),
      listCustomerSubscriptions: vi.fn().mockResolvedValue([
        {
          id: "sub-1",
          status: "active",
          planKey: "default-plan",
        },
      ]),
    });

    const result = await provisionCustomer(admin, {
      clientId: "client",
      externalUserId: "sub",
      planKey: "default-plan",
    });

    expect(result.created.customer).toBe(false);
    expect(result.created.subscription).toBe(false);
    expect(result.subscriptionId).toBe("sub-1");
    expect(admin.createCustomer).not.toHaveBeenCalled();
    expect(admin.createSubscription).not.toHaveBeenCalled();
  });
});
