import { createHash } from "node:crypto";

export const CUSTOMER_KEY_PREFIX = "ch_";

const BASE32_ALPHABET = "abcdefghijklmnopqrstuvwxyz234567";

function encodeBase32LowerNoPadding(buffer) {
  let bits = 0;
  let value = 0;
  let output = "";
  for (const byte of buffer) {
    value = (value << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      output += BASE32_ALPHABET[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) {
    output += BASE32_ALPHABET[(value << (5 - bits)) & 31];
  }
  return output;
}

export function buildAuthId(tenantId, clientId, externalUserId) {
  return `${String(tenantId).trim()}:${String(clientId).trim()}:${String(externalUserId).trim()}`;
}

export function parseAuthId(authId) {
  const trimmed = String(authId || "").trim();
  const first = trimmed.indexOf(":");
  if (first <= 0 || first >= trimmed.length - 1) {
    return null;
  }
  const secondOffset = trimmed.slice(first + 1).indexOf(":");
  if (secondOffset <= 0) {
    return null;
  }
  const second = first + 1 + secondOffset;
  if (second >= trimmed.length - 1) {
    return null;
  }
  return {
    tenantId: trimmed.slice(0, first),
    clientId: trimmed.slice(first + 1, second),
    externalUserId: trimmed.slice(second + 1),
  };
}

export function parseLegacyAuthId(authId) {
  const trimmed = String(authId || "").trim();
  const first = trimmed.indexOf(":");
  if (first <= 0 || first >= trimmed.length - 1) {
    return null;
  }
  return {
    clientId: trimmed.slice(0, first),
    externalUserId: trimmed.slice(first + 1),
  };
}

export function buildCustomerKey(tenantId, clientId, externalUserId) {
  const authId = buildAuthId(tenantId, clientId, externalUserId);
  const digest = createHash("sha256").update(authId).digest();
  return `${CUSTOMER_KEY_PREFIX}${encodeBase32LowerNoPadding(digest)}`;
}
