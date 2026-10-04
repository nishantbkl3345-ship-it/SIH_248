"use client";

// Replaces the root layout when it fails, so it carries its own document and
// cannot rely on the app's styles.
export default function GlobalError({ retry }: { error: Error; retry: () => void }) {
  return (
    <html lang="en">
      <body style={{ fontFamily: "system-ui, sans-serif", padding: "4rem 1.5rem", textAlign: "center" }}>
        <title>Something went wrong</title>
        <h1 style={{ fontSize: "1.25rem" }}>Something went wrong</h1>
        <p>The application failed to load.</p>
        <button onClick={() => retry()} style={{ padding: "0.5rem 1rem", cursor: "pointer" }}>
          Try again
        </button>
      </body>
    </html>
  );
}
