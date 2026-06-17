/** Serialize Konnect deepObject query params (filter, page) for openapi-fetch. */
export function konnectQuerySerializer(
  query: Record<string, unknown>,
): string {
  const parts: string[] = [];

  const append = (prefix: string, value: unknown): void => {
    if (value === undefined || value === null) {
      return;
    }
    if (
      typeof value === "string" ||
      typeof value === "number" ||
      typeof value === "boolean"
    ) {
      parts.push(`${prefix}=${encodeURIComponent(String(value))}`);
      return;
    }
    if (typeof value === "object" && !Array.isArray(value)) {
      for (const [key, nested] of Object.entries(
        value as Record<string, unknown>,
      )) {
        append(`${prefix}[${key}]`, nested);
      }
    }
  };

  for (const [key, value] of Object.entries(query)) {
    append(key, value);
  }

  return parts.join("&");
}
