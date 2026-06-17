import createClient, { type Client } from "openapi-fetch";
import { KonnectApiError } from "./errors.js";
import { konnectQuerySerializer } from "./query-serializer.js";
import type { paths } from "./schema.gen.js";
import { normalizeKonnectMeteringUrl } from "./url.js";

export type KonnectClientOptions = {
  baseUrl: string;
  apiKey: string;
  fetch?: typeof fetch;
};

export type KonnectApiClient = Client<paths>;

export type KonnectClient = {
  readonly baseUrl: string;
  readonly api: KonnectApiClient;
  waitForHealthy(attempts?: number, delayMs?: number): Promise<void>;
};

export function createKonnectClient(opts: KonnectClientOptions): KonnectClient {
  const baseUrl = normalizeKonnectMeteringUrl(opts.baseUrl);
  const apiKey = opts.apiKey.trim();
  const fetchFn = opts.fetch ?? globalThis.fetch;

  const api = createClient<paths>({
    baseUrl,
    fetch: fetchFn,
    querySerializer: konnectQuerySerializer,
  });

  api.use({
    onRequest({ request }) {
      request.headers.set("Authorization", `Bearer ${apiKey}`);
      return request;
    },
    async onResponse({ response, request }) {
      if (!response.ok) {
        const body = await response.text();
        const url = new URL(request.url);
        throw new KonnectApiError(
          request.method,
          url.pathname,
          response.status,
          body,
        );
      }
      return response;
    },
  });

  return {
    baseUrl,
    api,
    waitForHealthy: (attempts = 15, delayMs = 2000) =>
      waitForHealthy(api, baseUrl, attempts, delayMs),
  };
}

export function createKonnectClientFromEnv(
  env: NodeJS.ProcessEnv = process.env,
): KonnectClient {
  const baseUrl = env.OPENMETER_URL?.trim();
  const apiKey = env.OPENMETER_API_KEY?.trim();
  if (!baseUrl) {
    throw new Error("OPENMETER_URL is required");
  }
  if (!apiKey) {
    throw new Error("OPENMETER_API_KEY is required");
  }
  return createKonnectClient({ baseUrl, apiKey });
}

async function waitForHealthy(
  api: KonnectApiClient,
  baseUrl: string,
  attempts: number,
  delayMs: number,
): Promise<void> {
  for (let i = 0; i < attempts; i++) {
    try {
      const { response } = await api.GET("/openmeter/meters", {
        params: { query: { page: { number: 1, size: 1 } } },
      });
      if (response.ok) {
        return;
      }
    } catch {
      // retry
    }
    await new Promise((resolve) => setTimeout(resolve, delayMs));
  }
  throw new Error(
    `Konnect Metering & Billing not reachable at ${baseUrl}`,
  );
}
