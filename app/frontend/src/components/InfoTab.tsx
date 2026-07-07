import { useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";
import { Service } from "../../bindings/github.com/0x666c6f/berth/app/internal/state";
import { useStore } from "../store";
import { errText } from "../types";

const unwrap = (e: any) => e?.data;

type LiveStats = { CPU?: string; Memory?: string; NetIO?: string; PIDs?: string };

// Pull the headline dollar figure out of a CostSummary blob (e.g.
// "Estimated cost: $0.1234"); fall back to the first non-empty line so an
// unexpected format still shows something.
function parseCost(out: string): string {
  if (!out) return "";
  const m = out.match(/estimated cost:\s*(\$?[\d.,]+)/i) ?? out.match(/(\$[\d.,]+)/);
  if (m) return m[1].startsWith("$") ? m[1] : `$${m[1]}`;
  return out.trim().split("\n")[0] ?? "";
}

export function InfoTab({ name }: { name: string }) {
  const { agents, toast } = useStore();
  const a = agents.find((x) => x.Name === name);
  const [cost, setCost] = useState("");
  const [estCost, setEstCost] = useState("");
  const [live, setLive] = useState<LiveStats | null>(null);
  const [auditLines, setAuditLines] = useState<string[]>([]);

  useEffect(() => {
    Service.AuditTail(200)
      .then((entries: any[] | null) => setAuditLines(
        (entries ?? []).filter((e) => e.container === name)
          .map((e) => `${e.timestamp}  ${e.action}`)))
      .catch(() => setAuditLines([]));
  }, [name]);

  // Estimated cost on mount (binding added in parallel — guard).
  useEffect(() => {
    setEstCost("");
    const fn = (AgentService as any).CostSummary;
    if (typeof fn !== "function") return;
    fn(name).then((out: string) => setEstCost(parseCost(out))).catch(() => setEstCost(""));
  }, [name]);

  // Live resource stats stream (slower cadence than agents.changed); fall back
  // to the last poll snapshot when no stat has arrived yet.
  useEffect(() => {
    setLive(null);
    const off = Events.On("agents.stats", (e: any) => {
      const map = unwrap(e);
      const s = map?.[name];
      if (s) setLive(s);
    });
    return () => off();
  }, [name]);

  const cpu = live?.CPU ?? a?.CPU;
  const mem = live?.Memory ?? a?.Memory;
  const net = live?.NetIO ?? a?.NetIO;
  const pids = live?.PIDs ?? a?.PIDs;

  const fields: [string, string | undefined][] = [
    ["repo", a?.Repo], ["type", a?.Type], ["status", a?.Status],
    ["state", a?.State && `${a.State} — ${a.StateReason}`],
    ["fleet", a?.Fleet], ["hierarchy", a?.Hierarchy], ["network", a?.NetworkMode],
    ["ssh", a?.SSH], ["auth", a?.Auth],
    ["cpu", cpu], ["mem", mem], ["net", net], ["pids", pids],
    ["max cost", a?.MaxCost && (a.MaxCost.startsWith("$") ? a.MaxCost : `$${a.MaxCost}`)],
    ["est. cost", estCost],
  ];
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto p-4 text-sm">
      {a?.Prompt && (
        <div className="rounded bg-neutral-900 p-3">
          <div className="mb-1 text-xs uppercase text-neutral-500">task</div>
          <div className="whitespace-pre-wrap text-neutral-200">{a.Prompt}</div>
        </div>
      )}
      <table className="w-fit text-xs">
        <tbody>{fields.map(([k, v]) => (
          <tr key={k}><td className="pr-4 text-neutral-500">{k}</td><td>{v || "—"}</td></tr>
        ))}</tbody>
      </table>
      <div className="flex flex-wrap gap-2">
        <button className="btn" title="Push your current host Claude settings; applies on next agent restart"
          onClick={() => AgentService.ConfigSync(name, false)
            .then((o: string) => toast(o.trim())).catch((e: unknown) => toast(errText("config sync", e)))}>
          Refresh prefs
        </button>
        <button className="btn" title="Push settings and restart the session now (resumes conversation)"
          onClick={() => AgentService.ConfigSync(name, true)
            .then((o: string) => toast(o.trim())).catch((e: unknown) => toast(errText("config sync", e)))}>
          Refresh prefs & restart
        </button>
      </div>
      <div>
        <button className="btn" onClick={() =>
          AgentService.Cost(name).then(setCost).catch((e: unknown) => toast(errText("info", e)))}>
          Compute cost
        </button>
        {cost && <pre className="mt-2 whitespace-pre-wrap rounded bg-neutral-900 p-3">{cost}</pre>}
      </div>
      <div>
        <div className="mb-1 text-xs uppercase text-neutral-500">audit</div>
        <pre className="max-h-64 overflow-y-auto rounded bg-neutral-900 p-3 text-xs">{auditLines.join("\n") || "no entries"}</pre>
      </div>
    </div>
  );
}
