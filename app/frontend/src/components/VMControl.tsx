import { useState } from "react";
import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { useStore } from "../store";
import { errText } from "../types";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";

type VMAction = { key: string; label: string; confirm: boolean; danger?: boolean; exec: () => Promise<string> };

const ACTIONS: VMAction[] = [
  { key: "start", label: "Start", confirm: false, exec: () => AgentService.VMStart() },
  { key: "stop", label: "Stop", confirm: true, danger: true, exec: () => AgentService.VMStop() },
  { key: "restart", label: "Restart", confirm: true, danger: true, exec: () => AgentService.VMRestart() },
  { key: "repair", label: "Repair", confirm: true, exec: () => AgentService.VMRepair() },
];

// VMControl is the app-wide bottom status bar: VM health chip at the right,
// expanding into start/stop/restart/repair. When the VM is unreachable the
// whole bar turns red and carries the error. Stop and Restart kill running
// agents and Repair re-runs full setup, so those three arm on first click
// ("sure?") and fire on the second.
export function VMControl() {
  const { vmOk, vmError, run } = useStore();
  const [open, setOpen] = useState(false);
  const [armed, setArmed] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [diag, setDiag] = useState<string | null>(null);
  const [diagBusy, setDiagBusy] = useState(false);

  const fire = async (a: VMAction) => {
    setArmed(null);
    setBusy(a.key);
    try { await run(`VM ${a.label.toLowerCase()}`, a.exec()); }
    catch { /* run() already surfaced the error toast */ }
    finally { setBusy(null); }
  };

  // Diagnose runs the environment checklist (berth diagnose) and shows the
  // raw output in a dismissable panel — no toast, since it's multi-line.
  const diagnose = async () => {
    const fn = (AgentService as any).Diagnose;
    if (typeof fn !== "function") { setDiag("Diagnose is unavailable in this build."); return; }
    setDiagBusy(true);
    try { setDiag((await fn()) || "(no output)"); }
    catch (e) { setDiag(errText("diagnose", e)); }
    finally { setDiagBusy(false); }
  };

  return (
    <>
    {diag != null && (
      <div className="max-h-64 overflow-y-auto border-t border-red-900 bg-neutral-950 px-4 py-2 font-mono text-xs text-neutral-300">
        <div className="mb-1 flex items-center justify-between">
          <span className="font-semibold uppercase tracking-wider text-neutral-500">Diagnostics</span>
          <button aria-label="Dismiss diagnostics" className="text-neutral-500 hover:text-neutral-200" onClick={() => setDiag(null)}>
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
        <pre className="whitespace-pre-wrap">{diag}</pre>
      </div>
    )}
    <div className={`flex items-center justify-end gap-1.5 border-t px-3 py-1 text-xs ${vmOk ? "border-neutral-800 bg-neutral-900" : "border-red-900 bg-red-950/80"}`}>
      {!vmOk && <span className="min-w-0 flex-1 truncate text-red-300">VM unreachable: {vmError}</span>}
      {!vmOk && (
        <button disabled={diagBusy} title="Run berth diagnose (environment checklist)"
          className="rounded bg-neutral-800 px-2 py-0.5 text-neutral-300 hover:bg-neutral-700 disabled:opacity-40"
          onClick={diagnose}>
          {diagBusy ? "diagnosing…" : "Diagnose"}
        </button>
      )}
      {open && ACTIONS.map((a) => (
        <button key={a.key} disabled={!!busy}
          title={a.key === "repair" ? "Re-run berth setup (re-harden, reconcile Docker/NAT)" : `berth vm ${a.key}`}
          className={`rounded px-2 py-0.5 disabled:opacity-40 ${armed === a.key ? "bg-red-800 text-red-100" : a.danger ? "bg-neutral-800 text-neutral-300 hover:bg-red-900/60" : "bg-neutral-800 text-neutral-300 hover:bg-neutral-700"}`}
          onClick={() => (a.confirm && armed !== a.key ? setArmed(a.key) : fire(a))}>
          {armed === a.key ? "sure?" : a.label}
        </button>
      ))}
      <button className="flex shrink-0 items-center gap-1.5" title={vmOk ? "VM running — click for controls" : vmError}
        onClick={() => { setOpen((o) => !o); setArmed(null); }}>
        {open ? <ChevronRight className="h-3 w-3 text-neutral-600" /> : <ChevronLeft className="h-3 w-3 text-neutral-600" />}
        <span className={`h-2 w-2 rounded-full ${busy ? "animate-pulse bg-amber-400" : vmOk ? "bg-green-500" : "bg-red-500"}`} />
        <span className="font-semibold uppercase tracking-wider text-neutral-500">VM</span>
        <span className={vmOk ? "text-neutral-500" : "text-red-300"}>
          {busy ? `${busy}ing…` : vmOk ? "running" : "down"}
        </span>
      </button>
    </div>
    </>
  );
}
