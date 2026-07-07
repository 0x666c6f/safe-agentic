import { useEffect, useState } from "react";
import { ChevronDown, ChevronRight, TerminalSquare } from "lucide-react";
import { Events } from "@wailsio/runtime";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";

const unwrap = (e: any) => e?.data;
const CAP = 100;

// One executed berth CLI invocation. Shape mirrors the `cli.exec` event and
// AgentService.CommandLog() backlog (both added Go-side in parallel).
type Cmd = { ts?: string | number; argv?: string[]; ok?: boolean; durationMs?: number; tail?: string };

const clock = (ts?: string | number): string => {
  if (ts == null) return "";
  const t = typeof ts === "number" ? ts : Date.parse(ts);
  if (!t || Number.isNaN(t)) return typeof ts === "string" ? ts : "";
  return new Date(t).toTimeString().slice(0, 8); // HH:MM:SS
};

// ConsolePane surfaces the berth commands the app runs on your behalf — a
// collapsible "CLI activity" log so nothing happens invisibly. Newest first,
// capped so a long-lived window can't grow unbounded.
export function ConsolePane() {
  const [open, setOpen] = useState(false);
  const [cmds, setCmds] = useState<Cmd[]>([]);

  useEffect(() => {
    // CommandLog() may be absent until its binding lands — guard, don't crash.
    const backlog = (AgentService as any).CommandLog;
    if (typeof backlog === "function") {
      backlog()
        .then((log: Cmd[] | null) => setCmds((log ?? []).slice().reverse().slice(0, CAP)))
        .catch(() => {});
    }
    const off = Events.On("cli.exec", (e: any) => {
      const c = unwrap(e) as Cmd | undefined;
      if (!c) return;
      setCmds((prev) => [c, ...prev].slice(0, CAP));
    });
    return () => off();
  }, []);

  return (
    <div className="border-t border-neutral-800 text-xs">
      <button onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-3 py-2 text-neutral-400 hover:bg-neutral-900">
        {open ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        <TerminalSquare className="h-3.5 w-3.5" />
        <span className="font-medium">CLI activity</span>
        <span className="ml-auto tabular-nums text-neutral-600">{cmds.length}</span>
      </button>
      {open && (
        <div className="max-h-56 overflow-y-auto px-3 pb-2 font-mono">
          {cmds.map((c, i) => (
            <div key={i} className="flex items-center gap-2 border-b border-neutral-900 py-1 last:border-0">
              <span className="shrink-0 text-neutral-600">{clock(c.ts)}</span>
              <span className={`shrink-0 ${c.ok === false ? "text-red-400" : "text-green-400"}`}>{c.ok === false ? "✗" : "✓"}</span>
              <span className="min-w-0 flex-1 truncate text-neutral-300" title={(c.argv ?? []).join(" ")}>
                {(c.argv ?? []).join(" ") || c.tail || "—"}
              </span>
              {c.durationMs != null && <span className="shrink-0 tabular-nums text-neutral-600">{c.durationMs}ms</span>}
            </div>
          ))}
          {cmds.length === 0 && <div className="py-2 text-neutral-600">no CLI activity yet</div>}
        </div>
      )}
    </div>
  );
}
