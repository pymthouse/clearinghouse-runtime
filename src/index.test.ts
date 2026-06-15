import { describe, expect, it } from "vitest";
import { createApp } from "./index.js";

describe("createApp", () => {
  it("responds to /healthz", async () => {
    const server = createApp().listen(0);
    const { port } = server.address() as { port: number };

    const res = await fetch(`http://127.0.0.1:${port}/healthz`);
    const body = await res.json();

    expect(res.status).toBe(200);
    expect(body).toEqual({ status: "ok" });

    server.close();
  });

  it("returns 404 for unknown routes", async () => {
    const server = createApp().listen(0);
    const { port } = server.address() as { port: number };

    const res = await fetch(`http://127.0.0.1:${port}/unknown`);

    expect(res.status).toBe(404);

    server.close();
  });
});
