import { useEffect, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";
import { useStore } from "../store";
import { errText } from "../types";

// `berth cost --history` emits an aggregate summary (Period / Since / Spawns /
// Containers), NOT a per-agent (name, tokens, $) breakdown — per-session cost
// "requires live container access" and isn't in this output. So there's nothing
// per-agent to click through to; we parse the "key: value" lines into a tidy
// table and keep the raw text behind a toggle. Lines without a colon (e.g. the
// trailing note) are surfaced as-is below the table.
function parse(out: string): { rows: [string, string][]; notes: string[] } {
  const rows: [string, string][] = [];
  const notes: string[] = [];
  for (const line of out.split("\n")) {
    const t = line.trim();
    if (!t) continue;
    const m = t.match(/^([A-Za-z][\w .]*?):\s+(.*)$/);
    if (m) rows.push([m[1], m[2]]);
    else notes.push(t);
  }
  return { rows, notes };
}

export function CostView() {
  const toast = useStore((s) => s.toast);
  const [window, setWindow] = useState("7d");
  const [out, setOut] = useState("");
  const [raw, setRaw] = useState(false);

  useEffect(() => {
    AgentService.CostHistory(window).then(setOut).catch((e: unknown) => { setOut(""); toast(errText("cost", e)); });
  }, [window]);

  const { rows, notes } = parse(out);

  return (
    <div className="flex flex-col gap-3 p-6">
      <div className="flex items-center gap-2">
        {["1d", "7d", "30d"].map((w) => (
          <button key={w} className={`btn ${window === w ? "bg-neutral-600" : ""}`} onClick={() => setWindow(w)}>{w}</button>
        ))}
        <label className="ml-auto flex items-center gap-1.5 text-xs text-neutral-500">
          <input type="checkbox" checked={raw} onChange={(e) => setRaw(e.target.checked)} /> raw
        </label>
      </div>
      {raw || rows.length === 0 ? (
        <pre className="whitespace-pre-wrap rounded bg-neutral-900 p-4 text-xs">{out || "no cost data"}</pre>
      ) : (
        <div className="rounded bg-neutral-900 p-4">
          <table className="text-sm">
            <tbody>
              {rows.map(([k, v]) => (
                <tr key={k}><td className="pr-6 text-neutral-500">{k}</td><td className="tabular-nums">{v}</td></tr>
              ))}
            </tbody>
          </table>
          {notes.map((n) => <div key={n} className="mt-2 text-xs text-neutral-600">{n}</div>)}
        </div>
      )}
      <div className="text-xs text-neutral-600">Note: engine pricing table is dated (pkg/cost) — treat numbers as lower bounds.</div>
    </div>
  );
}
