import { test } from "node:test";
import assert from "node:assert/strict";
import { createEnsureCustomerHook } from "./ensure.mjs";

function fakeFetch(calls, response = { ok: true, status: 200 }) {
  return async (url, init) => {
    calls.push({ url, init });
    return {
      ok: response.ok,
      status: response.status,
      text: async () => response.body ?? "",
    };
  };
}

const identity = { client_id: "app_acme", usage_subject: "user_1" };

test("ensures the customer with builder-sdk identity shape", async () => {
  const calls = [];
  const hook = createEnsureCustomerHook({
    url: "http://tenant-admin:8094/internal/customers/ensure",
    secret: "s3cret",
    fetch: fakeFetch(calls),
  });

  await hook({ identity });

  assert.equal(calls.length, 1);
  assert.equal(calls[0].init.method, "POST");
  assert.equal(calls[0].init.headers["X-Internal-Secret"], "s3cret");
  assert.deepEqual(JSON.parse(calls[0].init.body), {
    clientId: "app_acme",
    externalUserId: "user_1",
  });
});

test("caches successful ensures within the TTL", async () => {
  const calls = [];
  const hook = createEnsureCustomerHook({
    url: "http://tenant-admin:8094/internal/customers/ensure",
    ttlSeconds: 600,
    fetch: fakeFetch(calls),
  });

  await hook({ identity });
  await hook({ identity });
  await hook({ identity });

  assert.equal(calls.length, 1, "second/third calls should be served from cache");
});

test("fail-closed: a non-2xx ensure throws to block signing", async () => {
  const hook = createEnsureCustomerHook({
    url: "http://tenant-admin:8094/internal/customers/ensure",
    fetch: fakeFetch([], { ok: false, status: 500, body: "boom" }),
  });

  await assert.rejects(() => hook({ identity }), /customer ensure returned 500/);
});

test("fail-open: a non-2xx ensure is tolerated when configured", async () => {
  const hook = createEnsureCustomerHook({
    url: "http://tenant-admin:8094/internal/customers/ensure",
    failOpen: true,
    fetch: fakeFetch([], { ok: false, status: 503 }),
  });

  await assert.doesNotReject(() => hook({ identity }));
});

test("no-ops when identity is incomplete", async () => {
  const calls = [];
  const hook = createEnsureCustomerHook({
    url: "http://tenant-admin:8094/internal/customers/ensure",
    fetch: fakeFetch(calls),
  });

  await hook({ identity: { client_id: "app_acme", usage_subject: "" } });

  assert.equal(calls.length, 0);
});
