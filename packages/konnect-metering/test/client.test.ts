import { describe, expect, it, vi } from "vitest";
import { createKonnectClient } from "../src/client.js";
import { KonnectApiError } from "../src/errors.js";
import { konnectQuerySerializer } from "../src/query-serializer.js";

const BASE = "https://us.api.konghq.com/v3";

describe("konnectQuerySerializer", () => {
  it("serializes deepObject filter and page params", () => {
    const query = konnectQuerySerializer({
      filter: { key: "client:sub", customer_id: "cust-1" },
      page: { number: 2, size: 50 },
    });
    expect(query).toContain("filter[key]=client%3Asub");
    expect(query).toContain("filter[customer_id]=cust-1");
    expect(query).toContain("page[number]=2");
    expect(query).toContain("page[size]=50");
  });
});

describe("createKonnectClient", () => {
  it("normalizes base URL to v3 root", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: [] }), { status: 200 }),
    );
    const client = createKonnectClient({
      baseUrl: "https://us.api.konghq.com/v3/openmeter",
      apiKey: "kpat_test",
      fetch: fetchFn,
    });

    await client.api.GET("/openmeter/meters");

    expect(client.baseUrl).toBe("https://us.api.konghq.com/v3");
    const request = fetchFn.mock.calls[0]?.[0] as Request;
    expect(request.url).toBe("https://us.api.konghq.com/v3/openmeter/meters");
  });

  it("throws KonnectApiError on non-OK responses", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      new Response("not found", { status: 404 }),
    );
    const client = createKonnectClient({
      baseUrl: BASE,
      apiKey: "kpat_test",
      fetch: fetchFn,
    });

    await expect(client.api.GET("/openmeter/meters")).rejects.toBeInstanceOf(
      KonnectApiError,
    );
  });

  it("sends Authorization bearer header", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ data: [] }), { status: 200 }),
    );
    const client = createKonnectClient({
      baseUrl: BASE,
      apiKey: "kpat_secret",
      fetch: fetchFn,
    });

    await client.api.GET("/openmeter/meters");

    const request = fetchFn.mock.calls[0]?.[0] as Request;
    expect(request.headers.get("Authorization")).toBe("Bearer kpat_secret");
  });

  it("retries waitForHealthy until meters endpoint responds", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValueOnce(new Response("down", { status: 503 }))
      .mockResolvedValue(
        new Response(JSON.stringify({ data: [] }), { status: 200 }),
      );
    const client = createKonnectClient({
      baseUrl: BASE,
      apiKey: "kpat_test",
      fetch: fetchFn,
    });

    await client.waitForHealthy(3, 1);

    expect(fetchFn).toHaveBeenCalledTimes(2);
  });
});
