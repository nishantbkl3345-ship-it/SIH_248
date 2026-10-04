import { describe, expect, it } from "vitest";

import { parseEnv } from "@/lib/env";

describe("parseEnv", () => {
  it("applies defaults", () => {
    expect(parseEnv({})).toEqual({
      API_URL: "http://localhost:8080",
      NEXT_PUBLIC_APP_NAME: "FOGLINE",
    });
  });

  it("strips trailing slashes from API_URL", () => {
    expect(parseEnv({ API_URL: "https://api.example.test/" }).API_URL).toBe("https://api.example.test");
  });

  it("rejects a malformed or non-http API_URL", () => {
    expect(() => parseEnv({ API_URL: "localhost:8080" })).toThrow(/API_URL/);
    expect(() => parseEnv({ API_URL: "ftp://example.test" })).toThrow(/API_URL/);
  });
});
