/**
 * Minimal OpenMeter balance + usage reads for identity-webhook usage/me routes.
 * Customer key format: {clientId}:{externalUserId}
 */

const NETWORK_FEE_USD_MICROS_METER = "network_fee_usd_micros";
const SIGNED_TICKET_COUNT_METER = "signed_ticket_count";

const METER_GROUP_BY_USER = ["client_id", "external_user_id"];
const METER_GROUP_BY_DETAIL = [
  "client_id",
  "external_user_id",
  "pipeline",
  "model_id",
];

function buildOpenMeterCustomerKey(clientId, externalUserId) {
  return `${clientId.trim()}:${externalUserId.trim()}`;
}

function normalizeBaseUrl(url) {
  let base = url.trim().replace(/\/$/, "");
  if (base.endsWith("/events")) {
    base = base.slice(0, -"/events".length);
  }
  if (!base.endsWith("/openmeter") && /\/v\d+$/i.test(base)) {
    base = `${base}/openmeter`;
  }
  return base;
}

function isKonnectMeteringUrl(url, apiKey) {
  if (/konghq\.com/i.test(url)) {
    return true;
  }
  const key = apiKey?.trim() ?? "";
  return key.startsWith("kpat_") || key.startsWith("spat_");
}

function mapKonnectGranularity(windowSize) {
  switch (windowSize?.trim().toUpperCase()) {
    case "MONTH":
      return "P1M";
    case "DAY":
      return "P1D";
    default:
      return undefined;
  }
}

function normalizeEntitlementValue(body) {
  if (!body || typeof body !== "object") {
    return null;
  }
  const record = body;
  return {
    hasAccess: record.hasAccess ?? record.has_access,
    balance: record.balance,
    usage: record.usage,
    totalAvailableGrantAmount:
      record.totalAvailableGrantAmount ?? record.total_available_grant_amount,
  };
}

function normalizeMeterQueryResponse(body) {
  if (!body || typeof body !== "object" || !Array.isArray(body.data)) {
    return [];
  }
  return body.data.map((row) => {
    if (!row || typeof row !== "object") {
      return row;
    }
    const item = { ...row };
    if (item.dimensions && typeof item.dimensions === "object" && !item.groupBy) {
      item.groupBy = item.dimensions;
    }
    if (item.from && !item.windowStart) {
      item.windowStart = item.from;
    }
    if (typeof item.value === "string" && item.value.trim() !== "") {
      const parsed = Number(item.value);
      if (Number.isFinite(parsed)) {
        item.value = parsed;
      }
    }
    return item;
  });
}

function groupByString(group, key, fallback) {
  const value = group[key];
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  return fallback;
}

function clientIdFromGroup(group, fallbackClientId) {
  return groupByString(group, "client_id", fallbackClientId);
}

function buildKonnectMeterQueryBody(input) {
  const body = {
    group_by_dimensions: [...input.groupBy],
  };
  if (input.startDate) {
    body.from = input.startDate;
  }
  if (input.endDate) {
    body.to = input.endDate;
  }
  const granularity = mapKonnectGranularity(input.windowSize);
  if (granularity) {
    body.granularity = granularity;
  }
  if (input.externalUserId) {
    body.filters = {
      dimensions: {
        subject: {
          eq: buildOpenMeterCustomerKey(input.clientId, input.externalUserId),
        },
      },
    };
  }
  return body;
}

function getUtcCalendarMonthIsoBounds(now = new Date()) {
  const y = now.getUTCFullYear();
  const m = now.getUTCMonth();
  const start = new Date(Date.UTC(y, m, 1, 0, 0, 0, 0));
  const end = new Date(Date.UTC(y, m + 1, 0, 23, 59, 59, 999));
  return { startDate: start.toISOString(), endDate: end.toISOString() };
}

function createOpenMeterClient(env) {
  const apiKey = env.OPENMETER_API_KEY?.trim();
  const rawBaseUrl = env.OPENMETER_URL?.trim();
  if (!apiKey || !rawBaseUrl) {
    return null;
  }

  const useKonnect = isKonnectMeteringUrl(rawBaseUrl, apiKey);
  const baseUrl = normalizeBaseUrl(rawBaseUrl);
  const trialFeatureKey =
    env.OPENMETER_TRIAL_FEATURE_KEY?.trim() || "network_spend";

  async function request(path, options = {}) {
    const url = new URL(path.replace(/^\//, ""), `${baseUrl}/`);
    const headers = new Headers(options.headers);
    headers.set("Authorization", `Bearer ${apiKey}`);
    if (options.body) {
      headers.set("Content-Type", "application/json");
    }

    const response = await fetch(url, {
      method: options.method || "GET",
      headers,
      body: options.body ? JSON.stringify(options.body) : undefined,
    });

    const contentType = response.headers.get("content-type") || "";
    const isJson = contentType.includes("application/json");
    const payload = isJson ? await response.json() : null;

    if (!response.ok) {
      const err = new Error(
        `OpenMeter ${options.method || "GET"} ${url.pathname} failed: ${response.status}`,
      );
      err.status = response.status;
      err.body = payload;
      throw err;
    }

    return payload;
  }

  async function getEntitlementValue(customerKey, featureKey) {
    const path = `/customers/${encodeURIComponent(customerKey)}/entitlements/${encodeURIComponent(featureKey)}/value`;
    try {
      const body = await request(path);
      return normalizeEntitlementValue(body);
    } catch (err) {
      if (err.status === 404) {
        return null;
      }
      throw err;
    }
  }

  async function queryMeter(meterSlug, input) {
    const path = `/meters/${encodeURIComponent(meterSlug)}/query`;
    const body = buildKonnectMeterQueryBody(input);
    const response = await request(path, { method: "POST", body });
    return normalizeMeterQueryResponse(response);
  }

  return {
    baseUrl,
    useKonnect,
    trialFeatureKey,
    getEntitlementValue,
    queryMeter,
  };
}

async function getTrialCreditBalance(client, input) {
  const customerKey = buildOpenMeterCustomerKey(input.clientId, input.externalUserId);
  const featureKey = input.featureKey || client.trialFeatureKey;
  const value = await client.getEntitlementValue(customerKey, featureKey);

  if (!value) {
    return {
      hasAccess: false,
      balanceUsdMicros: "0",
      consumedUsdMicros: "0",
      lifetimeGrantedUsdMicros: "0",
    };
  }

  const balance = Math.max(0, Math.floor(value.balance ?? 0));
  const usage = Math.max(0, Math.floor(value.usage ?? 0));
  const granted = Math.max(
    0,
    Math.floor(value.totalAvailableGrantAmount ?? balance + usage),
  );

  return {
    hasAccess: Boolean(value.hasAccess) && balance > 0,
    balanceUsdMicros: String(balance),
    consumedUsdMicros: String(usage),
    lifetimeGrantedUsdMicros: String(granted),
  };
}

function aggregateUserRows(input) {
  const countByUser = new Map();
  for (const row of input.countRows) {
    const group = row.groupBy || {};
    const externalUserId = groupByString(group, "external_user_id", "");
    if (!externalUserId) continue;
    if (clientIdFromGroup(group, input.clientId) !== input.clientId) continue;
    countByUser.set(
      externalUserId,
      (countByUser.get(externalUserId) ?? 0) + Math.floor(Number(row.value ?? 0)),
    );
  }

  const feeByUser = new Map();
  for (const row of input.feeRows) {
    const group = row.groupBy || {};
    const externalUserId = groupByString(group, "external_user_id", "");
    if (!externalUserId) continue;
    if (input.filterExternalUserId && externalUserId !== input.filterExternalUserId) {
      continue;
    }
    if (clientIdFromGroup(group, input.clientId) !== input.clientId) continue;
    feeByUser.set(
      externalUserId,
      (feeByUser.get(externalUserId) ?? 0n) + BigInt(Math.floor(Number(row.value ?? 0))),
    );
  }

  const externalUserIds = new Set([...countByUser.keys(), ...feeByUser.keys()]);
  const rows = [...externalUserIds].map((externalUserId) => ({
    externalUserId,
    requestCount: countByUser.get(externalUserId) ?? 0,
    networkFeeUsdMicros: (feeByUser.get(externalUserId) ?? 0n).toString(),
  }));

  if (rows.length === 0 && input.filterExternalUserId) {
    rows.push({
      externalUserId: input.filterExternalUserId,
      requestCount: countByUser.get(input.filterExternalUserId) ?? 0,
      networkFeeUsdMicros: "0",
    });
  }

  return rows;
}

function aggregatePipelineModelRows(input) {
  const countByKey = new Map();
  const metaByKey = new Map();

  for (const row of input.countRows) {
    const group = row.groupBy || {};
    if (clientIdFromGroup(group, input.clientId) !== input.clientId) continue;
    const pipeline = groupByString(group, "pipeline", "unknown");
    const modelId = groupByString(group, "model_id", "unknown");
    const key = `${pipeline}|${modelId}`;
    metaByKey.set(key, { pipeline, modelId });
    countByKey.set(
      key,
      (countByKey.get(key) ?? 0) + Math.floor(Number(row.value ?? 0)),
    );
  }

  const feeByKey = new Map();
  for (const row of input.feeRows) {
    const group = row.groupBy || {};
    if (clientIdFromGroup(group, input.clientId) !== input.clientId) continue;
    const pipeline = groupByString(group, "pipeline", "unknown");
    const modelId = groupByString(group, "model_id", "unknown");
    const key = `${pipeline}|${modelId}`;
    metaByKey.set(key, { pipeline, modelId });
    feeByKey.set(
      key,
      (feeByKey.get(key) ?? 0n) + BigInt(Math.floor(Number(row.value ?? 0))),
    );
  }

  const keys = new Set([...countByKey.keys(), ...feeByKey.keys()]);
  return [...keys].flatMap((key) => {
    const meta = metaByKey.get(key);
    if (!meta) {
      return [];
    }
    return [
      {
        pipeline: meta.pipeline,
        modelId: meta.modelId,
        requestCount: countByKey.get(key) ?? 0,
        networkFeeUsdMicros: (feeByKey.get(key) ?? 0n).toString(),
      },
    ];
  });
}

async function queryOpenMeterUsage(client, input) {
  const periodQuery = {
    clientId: input.clientId,
    startDate: input.startDate,
    endDate: input.endDate,
    windowSize: "MONTH",
    externalUserId: input.externalUserId,
    groupBy: METER_GROUP_BY_USER,
  };

  const [feeRows, countRows] = await Promise.all([
    client.queryMeter(NETWORK_FEE_USD_MICROS_METER, periodQuery),
    client.queryMeter(SIGNED_TICKET_COUNT_METER, periodQuery),
  ]);

  return aggregateUserRows({
    clientId: input.clientId,
    feeRows,
    countRows,
    filterExternalUserId: input.externalUserId,
  });
}

async function queryOpenMeterUserPipelineByModel(client, input) {
  const periodQuery = {
    clientId: input.clientId,
    startDate: input.startDate,
    endDate: input.endDate,
    windowSize: "MONTH",
    externalUserId: input.externalUserId,
    groupBy: METER_GROUP_BY_DETAIL,
  };

  const [feeRows, countRows] = await Promise.all([
    client.queryMeter(NETWORK_FEE_USD_MICROS_METER, periodQuery),
    client.queryMeter(SIGNED_TICKET_COUNT_METER, periodQuery),
  ]);

  return aggregatePipelineModelRows({
    clientId: input.clientId,
    feeRows,
    countRows,
  });
}

export function isUsageQueryEnabled(env) {
  return (
    env.USAGE_QUERY_ENABLED?.trim() === "1" &&
    Boolean(env.OPENMETER_URL?.trim()) &&
    Boolean(env.OPENMETER_API_KEY?.trim())
  );
}

export function createOpenMeterUsageReaders(env) {
  if (!isUsageQueryEnabled(env)) {
    return null;
  }

  const client = createOpenMeterClient(env);
  if (!client) {
    return null;
  }

  return {
    readBalance: async ({ clientId, externalUserId }) => {
      const balance = await getTrialCreditBalance(client, { clientId, externalUserId });
      return {
        externalUserId,
        ...balance,
        remainingUsdMicros: balance.balanceUsdMicros,
      };
    },
    readUsage: async ({ clientId, externalUserId, startDate, endDate }) => {
      const defaults = getUtcCalendarMonthIsoBounds();
      const periodStart = startDate || defaults.startDate;
      const periodEnd = endDate || defaults.endDate;

      const [usageRows, pipelineRows] = await Promise.all([
        queryOpenMeterUsage(client, {
          clientId,
          externalUserId,
          startDate: periodStart,
          endDate: periodEnd,
        }),
        queryOpenMeterUserPipelineByModel(client, {
          clientId,
          externalUserId,
          startDate: periodStart,
          endDate: periodEnd,
        }),
      ]);

      const userRow = usageRows.find((row) => row.externalUserId === externalUserId) ?? {
        externalUserId,
        requestCount: 0,
        networkFeeUsdMicros: "0",
      };

      return {
        clientId,
        period: { start: periodStart, end: periodEnd },
        currentUser: {
          externalUserId,
          requestCount: userRow.requestCount,
          currency: "USD",
          networkFeeUsdMicros: userRow.networkFeeUsdMicros,
          ownerChargeUsdMicros: userRow.networkFeeUsdMicros,
          endUserBillableUsdMicros: "0",
          pipelineModels: pipelineRows.map((row) => ({
            pipeline: row.pipeline,
            modelId: row.modelId,
            requestCount: row.requestCount,
            currency: "USD",
            networkFeeUsdMicros: row.networkFeeUsdMicros,
            ownerChargeUsdMicros: row.networkFeeUsdMicros,
            endUserBillableUsdMicros: "0",
          })),
        },
      };
    },
  };
}
