import { useEffect, useState } from "react";
import { Service } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/state";
import { useStore } from "../store";
import { errText } from "../types";

type Item = { timestamp: string; type: string; status: string; container: string; payload?: Record<string, string> };

const BADGE: Record<string, string> = {
  "needs-auth": "bg-yellow-700", stuck: "bg-yellow-700", blocked: "bg-yellow-700",
  failed: "bg-red-800", "failed-tests": "bg-red-800",
  "ready-for-review": "bg-blue-800", "ready-for-pr": "bg-blue-800", info: "bg-neutral-700",
};

export function Timeline() {
  const toast = useStore((s) => s.toast);
  const [items, setItems] = useState<Item[]>([]);
  const [inboxOnly, setInboxOnly] = useState(false);

  const reload = () => {
    const call = inboxOnly ? Service.Inbox(200) : Service.EventsTail(200);
    call.then((i: any) => setItems(([...(i ?? [])] as Item[]).reverse())).catch((e: unknown) => toast(errText("timeline", e)));
  };
  useEffect(reload, [inboxOnly]);

  return (
    <div className="flex h-full flex-col text-sm">
      <div className="flex items-center gap-3 border-b border-neutral-800 p-3">
        <label className="flex items-center gap-2 text-xs">
          <input type="checkbox" checked={inboxOnly} onChange={(e) => setInboxOnly(e.target.checked)} />
          needs-attention only
        </label>
        <button className="btn" onClick={reload}>Refresh</button>
        <span className="ml-auto text-xs text-neutral-600">{items.length} events</span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-3">
        {items.map((it, i) => (
          <div key={i} className="flex items-center gap-3 rounded bg-neutral-900 px-3 py-2">
            <span className={`shrink-0 rounded px-2 py-0.5 text-xs ${BADGE[it.status] ?? "bg-neutral-700"}`}>{it.status}</span>
            <span className="shrink-0 text-neutral-400">{it.container}</span>
            <span className="truncate">{it.payload?.message ?? it.type}</span>
            <span className="ml-auto shrink-0 text-xs text-neutral-600">{it.timestamp}</span>
          </div>
        ))}
        {items.length === 0 && <div className="text-neutral-500">no events</div>}
      </div>
    </div>
  );
}
