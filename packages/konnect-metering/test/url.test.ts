import { describe, expect, it } from "vitest";
import {
  DEFAULT_KONNECT_METERING_URL,
  isKonnectApiKey,
  isKonnectMeteringUrl,
  konnectIngestUrl,
  normalizeKonnectMeteringUrl,
} from "../src/url.js";

describe("normalizeKonnectMeteringUrl", () => {
  it("strips trailing slash and /openmeter suffix", () => {
    expect(
      normalizeKonnectMeteringUrl("https://us.api.konghq.com/v3/openmeter/"),
    ).toBe("https://us.api.konghq.com/v3");
  });

  it("strips /openmeter/events suffix", () => {
    expect(
      normalizeKonnectMeteringUrl(
        "https://us.api.konghq.com/v3/openmeter/events",
      ),
    ).toBe("https://us.api.konghq.com/v3");
  });

  it("keeps v3 root when already normalized", () => {
    expect(normalizeKonnectMeteringUrl("https://eu.api.konghq.com/v3")).toBe(
      "https://eu.api.konghq.com/v3",
    );
  });
});

describe("isKonnectApiKey", () => {
  it("detects kpat_ and spat_ prefixes", () => {
    expect(isKonnectApiKey("kpat_abc")).toBe(true);
    expect(isKonnectApiKey("spat_xyz")).toBe(true);
    expect(isKonnectApiKey("om_abc")).toBe(false);
  });
});

describe("isKonnectMeteringUrl", () => {
  it("detects konghq.com hostnames", () => {
    expect(isKonnectMeteringUrl("https://us.api.konghq.com/v3/openmeter")).toBe(
      true,
    );
  });

  it("falls back to api key prefix", () => {
    expect(isKonnectMeteringUrl("https://custom.example.com", "kpat_x")).toBe(
      true,
    );
  });
});

describe("konnectIngestUrl", () => {
  it("returns normalized base with /openmeter/events", () => {
    expect(konnectIngestUrl(DEFAULT_KONNECT_METERING_URL)).toBe(
      "https://us.api.konghq.com/v3/openmeter/events",
    );
    expect(
      konnectIngestUrl("https://us.api.konghq.com/v3/openmeter"),
    ).toBe("https://us.api.konghq.com/v3/openmeter/events");
  });
});
