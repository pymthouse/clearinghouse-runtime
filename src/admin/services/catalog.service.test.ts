import { describe, expect, it, vi } from "vitest";
import type { OpenMeterAdmin } from "../port.js";
import {
  bootstrapCatalog,
  ensureFeature,
  ensureMeter,
} from "./catalog.service.js";

function mockAdmin(
  overrides: Partial<OpenMeterAdmin> = {},
): OpenMeterAdmin {
  return {
    capabilities: { plans: "full", subscriptions: "full" },
    waitForHealthy: vi.fn().mockResolvedValue(undefined),
    listMeters: vi.fn().mockResolvedValue([]),
    createMeter: vi.fn().mockResolvedValue({
      id: "m-new",
      key: "test_meter",
    }),
    listFeatures: vi.fn().mockResolvedValue([]),
    createFeature: vi.fn().mockResolvedValue({
      id: "f-new",
      key: "test_feature",
      meterId: "m-1",
    }),
    listPlans: vi.fn().mockResolvedValue([]),
    ensurePlan: vi.fn().mockResolvedValue({
      id: "p-new",
      key: "default-plan",
    }),
    listCustomers: vi.fn().mockResolvedValue([]),
    createCustomer: vi.fn(),
    listCustomerSubscriptions: vi.fn().mockResolvedValue([]),
    createSubscription: vi.fn(),
    ...overrides,
  };
}

describe("ensureMeter", () => {
  it("returns existing meter without creating", async () => {
    const admin = mockAdmin({
      listMeters: vi.fn().mockResolvedValue([
        {
          id: "m-1",
          key: "network_fee_usd_micros",
          dimensions: { pipeline: "x", model_id: "y" },
        },
      ]),
    });

    const result = await ensureMeter(admin, {
      key: "network_fee_usd_micros",
      name: "Network",
      eventType: "evt",
      aggregation: "sum",
      dimensions: { pipeline: "x", model_id: "y" },
    });

    expect(result.created).toBe(false);
    expect(result.resource.id).toBe("m-1");
    expect(admin.createMeter).not.toHaveBeenCalled();
  });

  it("creates meter when missing", async () => {
    const admin = mockAdmin();

    const result = await ensureMeter(admin, {
      key: "new_meter",
      name: "New",
      eventType: "evt",
      aggregation: "count",
      dimensions: {},
    });

    expect(result.created).toBe(true);
    expect(admin.createMeter).toHaveBeenCalledOnce();
  });
});

describe("ensureFeature", () => {
  it("skips create when feature exists", async () => {
    const admin = mockAdmin({
      listFeatures: vi.fn().mockResolvedValue([
        { id: "f-1", key: "network_spend" },
      ]),
    });

    const result = await ensureFeature(admin, {
      key: "network_spend",
      name: "Network spend",
      meterId: "m-1",
    });

    expect(result.created).toBe(false);
    expect(admin.createFeature).not.toHaveBeenCalled();
  });
});

describe("bootstrapCatalog", () => {
  it("is idempotent when catalog resources already exist", async () => {
    const admin = mockAdmin({
      listMeters: vi.fn().mockResolvedValue([
        {
          id: "m-net",
          key: "network_fee_usd_micros",
          dimensions: { pipeline: "p", model_id: "m" },
        },
        {
          id: "m-bill",
          key: "billable_usd_micros",
          dimensions: { pipeline: "p", model_id: "m" },
        },
        {
          id: "m-count",
          key: "signed_ticket_count",
          dimensions: { pipeline: "p", model_id: "m" },
        },
      ]),
      listFeatures: vi.fn().mockResolvedValue([
        { id: "f-net", key: "network_spend" },
        { id: "f-bill", key: "billable_spend" },
      ]),
      listPlans: vi.fn().mockResolvedValue([
        { id: "p-1", key: "clearinghouse_default_ppu" },
      ]),
    });

    const result = await bootstrapCatalog(admin);

    expect(result.meters.every((m) => !m.created)).toBe(true);
    expect(result.features.every((f) => !f.created)).toBe(true);
    expect(result.plan?.created).toBe(false);
    expect(admin.createMeter).not.toHaveBeenCalled();
    expect(admin.ensurePlan).not.toHaveBeenCalled();
  });

  it("skips plan when backend has no plan capability", async () => {
    const admin = mockAdmin({
      capabilities: { plans: "none", subscriptions: "full" },
      listMeters: vi.fn().mockResolvedValue([
        {
          id: "m-net",
          key: "network_fee_usd_micros",
          dimensions: { pipeline: "p", model_id: "m" },
        },
        {
          id: "m-bill",
          key: "billable_usd_micros",
          dimensions: { pipeline: "p", model_id: "m" },
        },
        {
          id: "m-count",
          key: "signed_ticket_count",
          dimensions: { pipeline: "p", model_id: "m" },
        },
      ]),
      listFeatures: vi.fn().mockResolvedValue([
        { id: "f-net", key: "network_spend" },
        { id: "f-bill", key: "billable_spend" },
      ]),
    });

    const result = await bootstrapCatalog(admin);

    expect(result.plan).toBeUndefined();
    expect(result.planSkipped?.reason).toContain("not supported");
    expect(admin.ensurePlan).not.toHaveBeenCalled();
  });
});
