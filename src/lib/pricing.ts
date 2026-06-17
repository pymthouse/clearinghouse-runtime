import { readFileSync } from "node:fs";
import { dirname, isAbsolute, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export type MarkupRule = {
  pipeline: string;
  model_id: string;
  markupPercent: number;
};

export type PricingConfig = {
  defaultTrialIncludedUsdMicros: string;
  defaultPlanKey: string;
  billableFeatureKey: string;
  billableMeterKey: string;
  unitPriceUsdPerBillableMicro: string;
  markupRules: MarkupRule[];
};

function readPricingConfig(): PricingConfig {
  const moduleDir = dirname(fileURLToPath(import.meta.url));
  const configuredPath = process.env.PRICING_CONFIG_PATH?.trim();
  let configPath = resolve(moduleDir, "../../config/pricing.json");
  if (configuredPath) {
    configPath = isAbsolute(configuredPath)
      ? configuredPath
      : resolve(process.cwd(), configuredPath);
  }
  const rawConfig = readFileSync(configPath, "utf8");
  const config = JSON.parse(rawConfig) as PricingConfig;

  if (!config.defaultPlanKey?.trim()) {
    throw new Error("Invalid pricing config: defaultPlanKey is required");
  }
  if (!config.billableFeatureKey?.trim() || !config.billableMeterKey?.trim()) {
    throw new Error(
      "Invalid pricing config: billableFeatureKey and billableMeterKey are required",
    );
  }

  return config;
}

const pricingConfig = readPricingConfig();

export function getPricingConfig(): PricingConfig {
  return pricingConfig;
}

export function defaultTrialIncludedUsdMicros(): string {
  const envOverride = process.env.OPENMETER_DEFAULT_STARTER_INCLUDED_USD_MICROS?.trim();
  if (envOverride && /^\d+$/.test(envOverride)) {
    return envOverride;
  }
  const raw = pricingConfig.defaultTrialIncludedUsdMicros?.trim();
  if (raw && /^\d+$/.test(raw)) {
    return raw;
  }
  return "5000000";
}

export function defaultPlanKey(): string {
  const envOverride = process.env.OPENMETER_DEFAULT_PLAN_KEY?.trim();
  if (envOverride) {
    return envOverride;
  }
  return pricingConfig.defaultPlanKey.trim();
}

export function billableFeatureKey(): string {
  return pricingConfig.billableFeatureKey.trim();
}

export function billableMeterKey(): string {
  return pricingConfig.billableMeterKey.trim();
}

export function unitPriceUsdPerBillableMicro(): string {
  return pricingConfig.unitPriceUsdPerBillableMicro.trim() || "0.000001";
}

function ruleSpecificity(rule: MarkupRule): number {
  let score = 0;
  if (rule.pipeline !== "*") {
    score += 2;
  }
  if (rule.model_id !== "*") {
    score += 1;
  }
  return score;
}

/** Most-specific matching rule wins; falls back to wildcard pipeline and model at 0%. */
export function resolveMarkupPercent(
  pipeline: string,
  modelId: string,
): number {
  const p = pipeline.trim() || "unknown";
  const m = modelId.trim() || "unknown";

  let best: MarkupRule | null = null;
  let bestScore = -1;

  for (const rule of pricingConfig.markupRules) {
    const pipelineMatch =
      rule.pipeline === "*" || rule.pipeline === p;
    const modelMatch = rule.model_id === "*" || rule.model_id === m;
    if (!pipelineMatch || !modelMatch) {
      continue;
    }
    const score = ruleSpecificity(rule);
    if (score > bestScore) {
      best = rule;
      bestScore = score;
    }
  }

  if (!best) {
    return 0;
  }
  const pct = Number(best.markupPercent);
  return Number.isFinite(pct) && pct >= 0 ? pct : 0;
}

/** Apply markup percent to network USD micros (same semantics as builder-sdk retail). */
export function applyMarkupToNetworkMicros(
  networkFeeUsdMicros: number,
  markupPercent: number,
): number {
  const base = Math.max(0, Math.floor(networkFeeUsdMicros));
  const pct = Number.isFinite(markupPercent) ? Math.max(0, markupPercent) : 0;
  return Math.round(base * (1 + pct / 100));
}
