/**
 * afterVerify hook: ensure Konnect customer exists and check billable entitlement.
 */
import {
  billingUnavailableError,
  insufficientBalanceError,
} from "@pymthouse/builder-sdk/signer/webhook";

export function createBalanceGateHook({ gatewayUrl, gatewaySecret, enabled }) {
  if (!enabled) {
    return undefined;
  }

  const baseUrl = gatewayUrl.replace(/\/$/, "");

  return async ({ identity }) => {
    const headers = {
      "Content-Type": "application/json",
      Authorization: `Bearer ${gatewaySecret}`,
    };
    const body = {
      client_id: identity.client_id,
      external_user_id: identity.usage_subject,
    };

    const ensureRes = await fetch(`${baseUrl}/ensure`, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    });
    if (!ensureRes.ok) {
      throw billingUnavailableError("billing ensure failed");
    }

    const balanceUrl = new URL(`${baseUrl}/balance`);
    balanceUrl.searchParams.set("clientId", identity.client_id);
    balanceUrl.searchParams.set("externalUserId", identity.usage_subject);
    const balanceRes = await fetch(balanceUrl, { headers });
    if (!balanceRes.ok) {
      throw billingUnavailableError("billing balance lookup failed");
    }

    const balance = await balanceRes.json();
    if (!balance.hasAccess) {
      throw insufficientBalanceError();
    }
  };
}
