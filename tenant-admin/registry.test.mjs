import assert from "node:assert/strict";
import { mkdtemp, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";
import { createTenantRegistry } from "./registry.mjs";

describe("tenant registry", () => {
  it("provisions tenant and links auth0 client id", async () => {
    const dataDir = await mkdtemp(join(tmpdir(), "tenant-registry-"));
    const registry = createTenantRegistry({ dataDir });

    await registry.provisionTenant({
      tenantId: "acme",
      tenantName: "Acme Inc",
      clientId: "app_acme",
    });
    await registry.registerAuth0App({
      tenantId: "acme",
      clientId: "app_acme",
      auth0ClientId: "auth0-public-client",
    });

    const byAuth0 = await registry.lookupByAuth0ClientId("auth0-public-client");
    assert.equal(byAuth0?.tenantId, "acme");
    assert.equal(byAuth0?.clientId, "app_acme");

    const map = await registry.loadClientTenantMap();
    assert.equal(map.get("app_acme"), "acme");
    assert.equal(map.get("auth0-public-client"), "acme");

    const appsRaw = await readFile(join(dataDir, "apps.json"), "utf8");
    assert.match(appsRaw, /auth0-public-client/);
  });
});
