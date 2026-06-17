import { describe, expect, it } from "vitest";
import {
  applyMarkupToNetworkMicros,
  resolveMarkupPercent,
} from "./pricing.js";

describe("resolveMarkupPercent", () => {
  it("returns 0 for unknown pipeline/model", () => {
    expect(resolveMarkupPercent("foo", "bar")).toBe(0);
  });

  it("returns pipeline-specific markup", () => {
    expect(resolveMarkupPercent("live-video-to-video", "unknown")).toBe(15);
  });

  it("prefers more specific rules over wildcard", () => {
    expect(resolveMarkupPercent("live-video-to-video", "daydream-video")).toBe(15);
  });
});

describe("applyMarkupToNetworkMicros", () => {
  it("applies markup percent to network micros", () => {
    expect(applyMarkupToNetworkMicros(1_000_000, 15)).toBe(1_150_000);
    expect(applyMarkupToNetworkMicros(1_000_000, 0)).toBe(1_000_000);
  });
});
