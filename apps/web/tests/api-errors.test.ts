import { describe, expect, it } from "vitest";

import { toApiError } from "@/lib/api/errors";

describe("toApiError", () => {
  it("reads the API error envelope", async () => {
    const res = Response.json(
      {
        error: {
          code: "VALIDATION_FAILED",
          message: "One or more fields are invalid.",
          details: { email: "must be a valid email address" },
          requestId: "req-1",
        },
      },
      { status: 422 },
    );
    const err = await toApiError(res);
    expect(err).toMatchObject({
      status: 422,
      code: "VALIDATION_FAILED",
      details: { email: "must be a valid email address" },
      requestId: "req-1",
    });
  });

  it("falls back when the body is not the envelope", async () => {
    const html = await toApiError(new Response("<html>Bad Gateway</html>", { status: 502 }));
    expect(html.code).toBe("UNEXPECTED_RESPONSE");
    expect(html.message).toMatch(/unavailable/);

    const other = await toApiError(Response.json({ nope: true }, { status: 400 }));
    expect(other.code).toBe("UNEXPECTED_RESPONSE");
    expect(other.details).toEqual({});
  });
});
