/**
 * LangDAG SDK Error Classes
 */

/**
 * Base error class for all LangDAG SDK errors
 */
export class LangDAGError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'LangDAGError';
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/**
 * Error thrown when the API returns an error response
 */
export class ApiError extends LangDAGError {
  public readonly status: number;
  public readonly statusText: string;
  public readonly body?: unknown;

  constructor(status: number, statusText: string, body?: unknown) {
    const message = body && typeof body === 'object' && 'error' in body
      ? String((body as { error: unknown }).error)
      : `HTTP ${status}: ${statusText}`;

    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
}

/**
 * Error thrown when authentication fails (401)
 */
export class UnauthorizedError extends ApiError {
  constructor(body?: unknown) {
    super(401, 'Unauthorized', body);
    this.name = 'UnauthorizedError';
  }
}

/**
 * Error thrown when a resource is not found (404)
 */
export class NotFoundError extends ApiError {
  constructor(body?: unknown) {
    super(404, 'Not Found', body);
    this.name = 'NotFoundError';
  }
}

/**
 * Error thrown when the request is malformed (400)
 */
export class BadRequestError extends ApiError {
  constructor(body?: unknown) {
    super(400, 'Bad Request', body);
    this.name = 'BadRequestError';
  }
}

/**
 * Error thrown when SSE parsing fails
 */
export class SSEParseError extends LangDAGError {
  public readonly rawData?: string;

  constructor(message: string, rawData?: string) {
    super(message);
    this.name = 'SSEParseError';
    this.rawData = rawData;
  }
}

/**
 * Error thrown when a network error occurs
 */
export class NetworkError extends LangDAGError {
  public readonly cause?: Error;

  constructor(message: string, cause?: Error) {
    super(message);
    this.name = 'NetworkError';
    this.cause = cause;
  }
}

/**
 * Factory function to create the appropriate error based on status code
 */
export function createApiError(status: number, statusText: string, body?: unknown): ApiError {
  switch (status) {
    case 400:
      return new BadRequestError(body);
    case 401:
      return new UnauthorizedError(body);
    case 404:
      return new NotFoundError(body);
    default:
      return new ApiError(status, statusText, body);
  }
}
