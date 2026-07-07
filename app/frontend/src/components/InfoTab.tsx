import { useEffect, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { Service } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/state";
import { useStore } from "../store";
import { errText } from "../types";

export function InfoTab({ name }: { name: string }) {
  const { agents, toast } = useStore();
  const a = agents.find((x) => x.Name === name);
  const [cost, setCost] = useState("");
  const [auditLines, setAuditLines] = useState<string[]>([]);

  useEffect(() => {
    Service.AuditTail(200)
      .then((entries: any[] | null) => setAuditLines(
        (entries ?? []).filter((e) => e.container === name)
          .map((e) => `${e.timestamp}  ${e.action}`)))
      .catch(() => setAuditLines([]));
  }, [name]);

  const fields: [string, string | undefined][] = [
    ["repo", a?.Repo], ["type", a?.Type], ["status", a?.Status],
    ["state", a?.State && `${a.State} — ${a.StateReason}`],
    ["fleet", a?.Fleet], ["hierarchy", a?.Hierarchy], ["network", a?.NetworkMode],
    ["ssh", a?.SSH], ["auth", a?.Auth], ["cpu", a?.CPU], ["mem", a?.Memory], ["pids", a?.PIDs],
  ];
  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto p-4 text-sm">
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
