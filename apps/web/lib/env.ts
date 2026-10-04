import { z } from "zod";

// Server-side environment. Parsed once at startup (and from next.config.ts),
// so a bad value fails the build or boot instead of a request.
const schema = z.object({
  // Base URL of the Go API as reachable from the Next.js server.
  API_URL: z
    .url({ protocol: /^https?$/, error: "API_URL must be an http(s) URL" })
    .default("http://localhost:8080")
    .transform((v) => v.replace(/\/+$/, "")),
  NEXT_PUBLIC_APP_NAME: z.string().trim().min(1).default("FOGLINE"),
});

export type Env = z.infer<typeof schema>;

export function parseEnv(source: Record<string, string | undefined>): Env {
  const result = schema.safeParse({
    API_URL: source.API_URL || undefined,
    NEXT_PUBLIC_APP_NAME: source.NEXT_PUBLIC_APP_NAME || undefined,
  });
  if (!result.success) {
    const problems = result.error.issues
      .map((i) => `  ${i.path.join(".")}: ${i.message}`)
      .join("\n");
    throw new Error(`Invalid environment variables:\n${problems}`);
  }
  return result.data;
}

export const env = parseEnv({
  API_URL: process.env.API_URL,
  NEXT_PUBLIC_APP_NAME: process.env.NEXT_PUBLIC_APP_NAME,
});
