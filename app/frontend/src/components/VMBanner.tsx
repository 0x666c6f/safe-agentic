import { useState } from "react";
import { useStore } from "../store";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";

export function VMBanner() {
  const { vmOk, vmError, toast } = useStore();
  const [busy, setBusy] = useState(false);
  if (vmOk) return null;
  return (
    <div className="flex items-center gap-3 bg-red-900 px-4 py-2 text-sm text-red-100">
      <span>VM unreachable: {vmError}</span>
      <button
        disabled={busy}
        className="rounded bg-red-700 px-2 py-0.5 hover:bg-red-600 disabled:opacity-50"
        onClick={async () => {
          setBusy(true);
          try { toast(await AgentService.VMStart()); }
          catch (e) { toast(String(e)); }
          finally { setBusy(false); }
        }}
      >
        Start VM
      </button>
    </div>
  );
}
