import { readFileSync } from "node:fs";
import { dirname, isAbsolute, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type MeterConfig = {
  createSignedTicketEventType: string;
  signedTicketEventSource: string;
  defaultTrialFeatureKey: string;
  defaultBillableFeatureKey: string;
  networkFeeUsdMicrosMeter: string;
  billableUsdMicrosMeter: string;
  signedTicketCountMeter: string;
  dimensions: Record<string, string>;
  meters: {
    networkFeeUsdMicros: {
      openmeterDescription: string;
      konnectName: string;
      konnectDescription: string;
      valueProperty: string;
    };
    billableUsdMicros: {
      openmeterDescription: string;
      konnectName: string;
      konnectDescription: string;
      valueProperty: string;
    };
    signedTicketCount: {
      openmeterDescription: string;
      konnectName: string;
      konnectDescription: string;
    };
  };
};

function readMeterConfig(): MeterConfig {
  const moduleDir = dirname(fileURLToPath(import.meta.url));
  const configuredPath = process.env.METERS_CONFIG_PATH?.trim();
  let configPath = resolve(moduleDir, "../../config/meters.json");
  if (configuredPath) {
    configPath = isAbsolute(configuredPath)
      ? configuredPath
      : resolve(process.cwd(), configuredPath);
  }
  const rawConfig = readFileSync(configPath, "utf8");
  const config = JSON.parse(rawConfig) as MeterConfig;

  if (!config.createSignedTicketEventType) {
    throw new Error(
      "Invalid meter config: createSignedTicketEventType is required",
    );
  }
  if (!config.networkFeeUsdMicrosMeter || !config.signedTicketCountMeter) {
    throw new Error(
      "Invalid meter config: networkFeeUsdMicrosMeter and signedTicketCountMeter are required",
    );
  }
  if (!config.billableUsdMicrosMeter) {
    throw new Error("Invalid meter config: billableUsdMicrosMeter is required");
  }

  return config;
}

const meterConfig = readMeterConfig();

export const CREATE_SIGNED_TICKET_EVENT_TYPE =
  meterConfig.createSignedTicketEventType;
export const SIGNED_TICKET_EVENT_SOURCE = meterConfig.signedTicketEventSource;
export const NETWORK_FEE_USD_MICROS_METER = meterConfig.networkFeeUsdMicrosMeter;
export const BILLABLE_USD_MICROS_METER = meterConfig.billableUsdMicrosMeter;
export const SIGNED_TICKET_COUNT_METER = meterConfig.signedTicketCountMeter;
export const DEFAULT_TRIAL_FEATURE_KEY = meterConfig.defaultTrialFeatureKey;
export const DEFAULT_BILLABLE_FEATURE_KEY =
  meterConfig.defaultBillableFeatureKey;

const LIVEPEER_DIMENSIONS = meterConfig.dimensions;

export const OPENMETER_METER_DEFINITIONS = [
  {
    slug: NETWORK_FEE_USD_MICROS_METER,
    description: meterConfig.meters.networkFeeUsdMicros.openmeterDescription,
    eventType: CREATE_SIGNED_TICKET_EVENT_TYPE,
    aggregation: "SUM" as const,
    valueProperty: meterConfig.meters.networkFeeUsdMicros.valueProperty,
    groupBy: { ...LIVEPEER_DIMENSIONS },
  },
  {
    slug: BILLABLE_USD_MICROS_METER,
    description: meterConfig.meters.billableUsdMicros.openmeterDescription,
    eventType: CREATE_SIGNED_TICKET_EVENT_TYPE,
    aggregation: "SUM" as const,
    valueProperty: meterConfig.meters.billableUsdMicros.valueProperty,
    groupBy: { ...LIVEPEER_DIMENSIONS },
  },
  {
    slug: SIGNED_TICKET_COUNT_METER,
    description: meterConfig.meters.signedTicketCount.openmeterDescription,
    eventType: CREATE_SIGNED_TICKET_EVENT_TYPE,
    aggregation: "COUNT" as const,
    groupBy: { ...LIVEPEER_DIMENSIONS },
  },
];

export const KONNECT_METER_DEFINITIONS = [
  {
    key: NETWORK_FEE_USD_MICROS_METER,
    name: meterConfig.meters.networkFeeUsdMicros.konnectName,
    description: meterConfig.meters.networkFeeUsdMicros.konnectDescription,
    event_type: CREATE_SIGNED_TICKET_EVENT_TYPE,
    aggregation: "sum" as const,
    value_property: meterConfig.meters.networkFeeUsdMicros.valueProperty,
    dimensions: { ...LIVEPEER_DIMENSIONS },
  },
  {
    key: BILLABLE_USD_MICROS_METER,
    name: meterConfig.meters.billableUsdMicros.konnectName,
    description: meterConfig.meters.billableUsdMicros.konnectDescription,
    event_type: CREATE_SIGNED_TICKET_EVENT_TYPE,
    aggregation: "sum" as const,
    value_property: meterConfig.meters.billableUsdMicros.valueProperty,
    dimensions: { ...LIVEPEER_DIMENSIONS },
  },
  {
    key: SIGNED_TICKET_COUNT_METER,
    name: meterConfig.meters.signedTicketCount.konnectName,
    description: meterConfig.meters.signedTicketCount.konnectDescription,
    event_type: CREATE_SIGNED_TICKET_EVENT_TYPE,
    aggregation: "count" as const,
    dimensions: { ...LIVEPEER_DIMENSIONS },
  },
];
