import { useState } from "react";
import { ChevronDown, ChevronUp } from "lucide-react";
import { useStore } from "../store";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";

type VMAction = { key: string; label: string; confirm: boolean; danger?: boolean; exec: () => Promise<string> };

const ACTIONS: VMAction[] = [
  { key: "start", label: "Start", confirm: false, exec: () => AgentService.VMStart() },
  { key: "stop", label: "Stop", confirm: true, danger: true, exec: () => AgentService.VMStop() },
  { key: "restart", label: "Restart", confirm: true, danger: true, exec: () => AgentService.VMRestart() },
  { key: "repair", label: "Repair", confirm: true, exec: () => AgentService.VMRepair() },
];

// VMControl is the always-visible VM health chip above the quota bars: status
// dot + one-line state, expanding to start/stop/restart/repair. Stop and
// Restart kill running agents and Repair re-runs full setup, so those three
// arm on first click ("sure?") and fire on the second.
export function VMControl() {
  const { vmOk, vmError, run } = useStore();
  const [open, setOpen] = useState(false);
  const [armed, setArmed] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const fire = async (a: VMAction) => {
    setArmed(null);
    setBusy(a.key);
    try { await run(`VM ${a.label.toLowerCase()}`, a.exec()); }
    catch { /* run() already surfaced the error toast */ }
    finally { setBusy(null); }
  };

  return (
    <div className="border-t border-neutral-800 px-3 py-2 text-xs">
      <button className="flex w-full items-center gap-2" title={vmOk ? "VM running" : vmError}
        onClick={() => { setOpen((o) => !o); setArmed(null); }}>
        <span className={`h-2 w-2 shrink-0 rounded-full ${busy ? "animate-pulse bg-amber-400" : vmOk ? "bg-green-500" : "bg-red-500"}`} />
        <span className="font-semibold uppercase tracking-wider text-neutral-600">VM</span>
        <span className={`min-w-0 flex-1 truncate text-left ${vmOk ? "text-neutral-500" : "text-red-400"}`}>
          {busy ? `${busy}ing…` : vmOk ? "running" : vmError || "unreachable"}
        </span>
        {open ? <ChevronDown className="h-3 w-3 shrink-0 text-neutral-600" /> : <ChevronUp className="h-3 w-3 shrink-0 text-neutral-600" />}
      </button>
      {open && (
        <div className="mt-1.5 flex gap-1">
          {ACTIONS.map((a) => (
            <button key={a.key} disabled={!!busy}
              title={a.key === "repair" ? "Re-run safe-ag setup (re-harden, reconcile Docker/NAT)" : `safe-ag vm ${a.key}`}
              className={`flex-1 rounded px-1 py-1 disabled:opacity-40 ${armed === a.key ? "bg-red-800 text-red-100" : a.danger ? "bg-neutral-800 text-neutral-300 hover:bg-red-900/60" : "bg-neutral-800 text-neutral-300 hover:bg-neutral-700"}`}
              onClick={() => (a.confirm && armed !== a.key ? setArmed(a.key) : fire(a))}>
              {armed === a.key ? "sure?" : a.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
