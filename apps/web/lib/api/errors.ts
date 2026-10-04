export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly details: Record<string, string> = {},
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Builds an ApiError from a non-2xx response in the API's error envelope. */
export async function toApiError(res: Response): Promise<ApiError> {
  try {
    const body: unknown = await res.json();
    const e = (body as { error?: Record<string, unknown> } | null)?.error;
    if (e && typeof e.code === "string" && typeof e.message === "string") {
      return new ApiError(
        res.status,
        e.code,
        e.message,
        (e.details as Record<string, string> | undefined) ?? {},
        typeof e.requestId === "string" ? e.requestId : undefined,
      );
    }
  } catch {
    // Not JSON: a proxy or gateway answered instead of the API.
  }
  return new ApiError(
    res.status,
    "UNEXPECTED_RESPONSE",
    res.status >= 500
      ? "The server is unavailable. Please try again shortly."
      : "The request could not be completed.",
  );
}
