export function buildAuthId(tenantId, clientId, externalUserId) {
  return `${String(tenantId).trim()}:${String(clientId).trim()}:${String(externalUserId).trim()}`;
}
