export class AdminError extends Error {
  constructor(
    message: string,
    readonly code: string,
  ) {
    super(message);
    this.name = "AdminError";
  }
}

export class BackendUnreachableError extends AdminError {
  constructor(message: string) {
    super(message, "BACKEND_UNREACHABLE");
    this.name = "BackendUnreachableError";
  }
}

export class ResourceNotFoundError extends AdminError {
  constructor(resource: string, id: string) {
    super(`${resource} not found: ${id}`, "NOT_FOUND");
    this.name = "ResourceNotFoundError";
  }
}

export class UnsupportedOperationError extends AdminError {
  constructor(operation: string) {
    super(`Operation not supported: ${operation}`, "UNSUPPORTED");
    this.name = "UnsupportedOperationError";
  }
}
