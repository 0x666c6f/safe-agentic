import { useEffect, useState } from "react";
import { QuotaService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";

type Win = { label: string; percent: number; resetsAt: number };
type Quota = { agent: string; ok: boolean; error: string; windows: Win[] };

// Bars show quota REMAINING (100 − used): green with headroom, red when nearly spent.
const remaining = (used: number) => Math.max(0, 100 - used);
const barColor = (left: number) => (left <= 10 ? "#ef4444" : left <= 30 ? "#f59e0b" : "#22c55e");

const resetIn = (unix: number) => {
  if (!unix) return "";
  const secs = unix - Math.floor(Date.now() / 1000);
  if (secs <= 0) return "resetting now";
  const h = Math.floor(secs / 3600), m = Math.floor((secs % 3600) / 60);
  return h > 0 ? `resets in ${h}h ${m}m` : `resets in ${m}m`;
};

// Compact countdown for the row itself: 34m, 2h05m, 5d.
const resetShort = (unix: number) => {
  if (!unix) return "";
  const secs = unix - Math.floor(Date.now() / 1000);
  if (secs <= 0) return "now";
  const d = Math.floor(secs / 86400);
  if (d > 0) return `${d}d`;
  const h = Math.floor(secs / 3600), m = Math.floor((secs % 3600) / 60);
  return h > 0 ? `${h}h${String(m).padStart(2, "0")}m` : `${m}m`;
};

export function QuotaBar() {
  const [quotas, setQuotas] = useState<Quota[]>([]);
  useEffect(() => {
    let live = true;
    const load = () => QuotaService.Quotas().then((q: any) => { if (live) setQuotas(q ?? []); }).catch(() => {});
    load();
    // Quotas move slowly and the Claude usage endpoint rate-limits — poll every
    // 5 min (the Go side also caches, so this never hammers the network).
    const t = setInterval(load, 5 * 60 * 1000);
    return () => { live = false; clearInterval(t); };
  }, []);
  if (!quotas.length) return null;

  return (
    <div className="border-t border-neutral-800 px-3 py-2">
      <div className="pb-1 text-[10px] font-semibold uppercase tracking-wider text-neutral-600">Quota left</div>
      {quotas.map((q) => (
        <div key={q.agent} className="mb-1.5 last:mb-0">
          <div className="flex items-baseline justify-between text-[11px]">
            <span className="capitalize text-neutral-400">{q.agent}</span>
            {!q.ok && <span className="text-neutral-600">{q.error || "unavailable"}</span>}
          </div>
          {q.ok && (
            <div className="mt-0.5 flex flex-col gap-0.5">
              {q.windows.map((w) => {
                const left = remaining(w.percent);
                return (
                  <div key={w.label} className="flex items-center gap-1.5 text-[10px]"
                    title={`${w.label}: ${left.toFixed(0)}% left (${w.percent.toFixed(0)}% used)${w.resetsAt ? " · " + resetIn(w.resetsAt) : ""}`}>
                    <span className="w-6 text-neutral-600">{w.label}</span>
                    <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-neutral-800">
                      <div className="h-full rounded-full" style={{ width: `${Math.max(2, left)}%`, backgroundColor: barColor(left) }} />
                    </div>
                    <span className="w-7 text-right tabular-nums text-neutral-500">{left.toFixed(0)}%</span>
                    <span className="w-11 text-right tabular-nums text-neutral-600">{resetShort(w.resetsAt)}</span>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
