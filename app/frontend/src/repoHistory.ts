// Saved repo list for the spawn form, sorted by most used.
// Backed by localStorage (per-app WKWebView storage); falls back to an
// in-memory map when localStorage is unavailable (unit tests).

const KEY = "safeag.repoHistory.v1";

type Entry = { count: number; last: number };
type History = Record<string, Entry>;

const mem: { data: string | null } = { data: null };

function readRaw(): string | null {
  try { return window.localStorage.getItem(KEY); } catch { return mem.data; }
}
function writeRaw(v: string) {
  try { window.localStorage.setItem(KEY, v); } catch { mem.data = v; }
}

function load(): History {
  const raw = readRaw();
  if (!raw) return {};
  try { return JSON.parse(raw) as History; } catch { return {}; }
}

export function recordRepoUse(url: string, now = Date.now()): void {
  const u = url.trim();
  if (!u) return;
  const h = load();
  const e = h[u] ?? { count: 0, last: 0 };
  h[u] = { count: e.count + 1, last: now };
  writeRaw(JSON.stringify(h));
}

export function topRepos(n = 5): string[] {
  const h = load();
  return Object.entries(h)
    .sort(([, a], [, b]) => b.count - a.count || b.last - a.last)
    .slice(0, n)
    .map(([url]) => url);
}

export function forgetRepo(url: string): void {
  const h = load();
  delete h[url];
  writeRaw(JSON.stringify(h));
}
