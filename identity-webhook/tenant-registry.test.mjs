import assert from "node:assert/strict";
import { mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, it } from "node:test";
import { buildAuthId } from "./billing-identity.mjs";
import { createTenantRegistryIndex } from "./tenant-registry.mjs";

describe("tenant registry index", () => {
  it("maps auth0 client id to billing client and tenant", async () => {
    const dir = await mkdtemp(join(tmpdir(), "tenant-registry-index-"));
    const registryPath = join(dir, "apps.json");
    await writeFile(
      registryPath,
      JSON.stringify([
        {
          tenantId: "demo",
          clientId: "demo-client",
          auth0ClientId: "auth0-public",
        },
      ]),
    );

    const index = createTenantRegistryIndex({
      TENANT_REGISTRY_PATH: registryPath,
    });

    assert.equal(index.resolveTenantId("auth0-public", "auth0-public"), "demo");
    assert.equal(index.billingClientId("auth0-public", "auth0-public"), "demo-client");
    assert.equal(
      buildAuthId("demo", "demo-client", "auth0|user-1"),
      "demo:demo-client:auth0|user-1",
    );
  });
});
