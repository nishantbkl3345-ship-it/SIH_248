const pad = (n: number) => String(n).padStart(2, "0");

/** Simulation clock: 150 -> "02:30", 3725 -> "1:02:05". */
export function formatClock(totalSec: number): string {
  const sec = Math.max(0, Math.round(totalSec));
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`;
}

/**
 * Parses what an instructor types into a time field: "2:30", "02:30",
 * "1:02:05", or a bare number of minutes ("5", "2.5"). Returns seconds, or
 * null if the text is not a time.
 */
export function parseClock(text: string): number | null {
  const t = text.trim();
  if (t === "") return null;

  if (/^\d+(\.\d+)?$/.test(t)) {
    return Math.round(Number(t) * 60);
  }
  const parts = t.split(":");
  if (parts.length < 2 || parts.length > 3 || !parts.every((p) => /^\d{1,3}$/.test(p))) {
    return null;
  }
  const nums = parts.map(Number);
  const [h, m, s] = nums.length === 3 ? nums : [0, nums[0], nums[1]];
  if (s > 59 || (nums.length === 3 && m > 59)) return null;
  return h * 3600 + m * 60 + s;
}

/** Human duration: 30 -> "30 s", 150 -> "2 min 30 s", 3600 -> "1 h". */
export function formatDuration(totalSec: number): string {
  const sec = Math.max(0, Math.round(totalSec));
  if (sec === 0) return "0 s";
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  return [h && `${h} h`, m && `${m} min`, s && `${s} s`].filter(Boolean).join(" ");
}
