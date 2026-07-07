import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { Service } from "../../bindings/github.com/0x666c6f/berth/app/internal/state";
import { useStore } from "../store";
import { errText } from "../types";
import type { AgentStatus } from "../types";
import { STATUS_COLORS } from "./StatusDot";
import { ConsolePane } from "./ConsolePane";
import { relTime } from "../relTime";

type Item = { timestamp: string; type: string; status: string; container: string; payload?: Record<string, string> };
type Row = Item & { _key: number };

const unwrap = (e: any) => e?.data;

// Statuses that count as "needs attention" — mirrors the server Inbox filter so
// live appends respect the needs-attention-only toggle.
const ATTENTION = new Set(["needs-auth", "stuck", "blocked", "failed", "failed-tests", "ready-for-review", "ready-for-pr"]);

// Map a raw event status onto the semantic AgentStatus buckets so the timeline
// draws from the single STATUS_COLORS source (StatusDot) instead of its own map.
function bucket(status: string): AgentStatus {
  if (status === "needs-auth" || status === "stuck" || status === "blocked") return "needs-you";
  if (status === "failed" || status === "failed-tests") return "failed";
  if (status === "ready-for-review" || status === "ready-for-pr") return "review";
  if (status === "working") return "working";
  return "idle";
}

let rowSeq = 0;
const keyed = (items: Item[]): Row[] => items.map((it) => ({ ...it, _key: ++rowSeq }));

export function Timeline() {
  const toast = useStore((s) => s.toast);
  const select = useStore((s) => s.select);
  const setView = useStore((s) => s.setView);
  const [items, setItems] = useState<Row[]>([]);
  const [inboxOnly, setInboxOnly] = useState(false);
  const [container, setContainer] = useState(""); // "" = all
  const [now, setNow] = useState(() => Date.now());

  const reload = useCallback(() => {
    const call = inboxOnly ? Service.Inbox(200) : Service.EventsTail(200);
    call.then((i: any) => setItems(keyed(([...(i ?? [])] as Item[]).reverse())))
      .catch((e: unknown) => toast(errText("timeline", e)));
  }, [inboxOnly, toast]);
  useEffect(() => { reload(); }, [reload]);

  // Live appends: prepend each new event without a full refetch.
  useEffect(() => {
    const off = Events.On("event.new", (e: any) => {
      const d = unwrap(e);
      const ev = d?.event;
      if (!ev) return;
      const status = d?.status ?? ev.status ?? "";
      if (inboxOnly && !ATTENTION.has(status)) return; // honour the active filter
      const item: Row = {
        _key: ++rowSeq,
        timestamp: ev.timestamp ?? "",
        type: ev.type ?? "",
        status,
        container: ev.payload?.container ?? ev.container ?? "",
        payload: ev.payload ?? undefined,
      };
      setItems((prev) => [item, ...prev].slice(0, 500));
    });
    return () => off();
  }, [inboxOnly]);

  // Refresh relative-time labels on a 30s tick (no network).
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30000);
    return () => clearInterval(t);
  }, []);

  const containers = [...new Set(items.map((i) => i.container).filter(Boolean))].sort();
  const shown = container ? items.filter((i) => i.container === container) : items;
  const open = (name: string) => { if (name) { select(name); setView("agents"); } };

  return (
    <div className="flex h-full flex-col text-sm">
      <div className="flex items-center gap-3 border-b border-neutral-800 p-3">
        <label className="flex items-center gap-2 text-xs">
          <input type="checkbox" checked={inboxOnly} onChange={(e) => setInboxOnly(e.target.checked)} />
          needs-attention only
        </label>
        {containers.length > 0 && (
          <select className="rounded bg-neutral-800 px-2 py-1 text-xs" value={container}
            onChange={(e) => setContainer(e.target.value)}>
            <option value="">all agents</option>
            {containers.map((c) => <option key={c} value={c}>{c.replace(/^agent-/, "")}</option>)}
          </select>
        )}
        <button className="btn" onClick={reload}>Refresh</button>
        <span className="ml-auto text-xs text-neutral-600">{shown.length} events</span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-3">
        {shown.map((it) => (
          <button key={it._key}
            onClick={() => open(it.container)}
            title={it.container ? `Open ${it.container.replace(/^agent-/, "")}` : undefined}
            className="flex items-center gap-3 rounded bg-neutral-900 px-3 py-2 text-left enabled:hover:bg-neutral-800 disabled:cursor-default"
            disabled={!it.container}>
            <span className="flex shrink-0 items-center gap-1.5">
              <span className={`h-2 w-2 rounded-full ${STATUS_COLORS[bucket(it.status)]}`} />
              <span className="text-xs text-neutral-400">{it.status}</span>
            </span>
            <span className="shrink-0 text-neutral-400">{it.container.replace(/^agent-/, "")}</span>
            <span className="truncate">{it.payload?.message ?? it.type}</span>
            <span className="ml-auto shrink-0 text-xs text-neutral-600" title={it.timestamp}>{relTime(it.timestamp, now)}</span>
          </button>
        ))}
        {shown.length === 0 && <div className="text-neutral-500">no events</div>}
      </div>
      <ConsolePane />
    </div>
  );
}
