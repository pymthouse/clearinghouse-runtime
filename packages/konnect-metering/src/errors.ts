export class KonnectApiError extends Error {
  readonly status: number;
  readonly path: string;
  readonly body: string;

  constructor(
    method: string,
    path: string,
    status: number,
    body: string,
  ) {
    super(
      `Konnect Metering API ${method} ${path} failed (${status}): ${body}`,
    );
    this.name = "KonnectApiError";
    this.status = status;
    this.path = path;
    this.body = body;
  }
}
