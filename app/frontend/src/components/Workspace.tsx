import { useEffect, useState } from "react";
import { CircleAlert, CircleX, ExternalLink, X } from "lucide-react";
import { useStore } from "../store";
import { TerminalPane } from "./TerminalPane";
import { OutputTab } from "./OutputTab";
import { InfoTab } from "./InfoTab";
import { DiffTab } from "./DiffTab";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";
import type { Tab } from "../types";

const TABS: Tab[] = ["terminal", "diff", "output", "info"];

export function Workspace({ name }: { name: string }) {
  const { splits, toggleSplit, agents, tab, setTab, run } = useStore();
  const me = agents.find((a) => a.Name === name);
  const others = agents.filter((a) => a.Running && a.Name !== name && !splits.includes(a.Name));
  const panes = splits.filter((n) => n !== name);
  const blocked = me?.Running && me.State === "blocked";
  // Failed = stopped with a non-clean exit (State "exited", not "done").
  const failed = me && !me.Running && me.State === "exited";
  const [failReason, setFailReason] = useState("");
  const [pr, setPr] = useState<{ url: string; number: number; state: string; title: string } | null>(null);

  useEffect(() => {
    if (!failed) { setFailReason(""); return; }
    AgentService.Output(name)
      .then((i: any) => setFailReason((i?.last_output || "").trim().split("\n").slice(-3).join("\n")))
      .catch(() => {});
  }, [name, failed]);

  // Look up an open PR for this agent's branch (gh in its workspace),
  // re-checking so it appears once the agent creates one mid-session.
  useEffect(() => {
    setPr(null);
    if (!me?.Running || !me.Repo) return;
    let cancelled = false;
    const check = () => AgentService.PRStatus(name, me.Repo!)
      .then((p: any) => { if (!cancelled) setPr(p?.url ? p : null); })
      .catch(() => {});
    check();
    const t = setInterval(check, 30000);
    return () => { cancelled = true; clearInterval(t); };
  }, [name, me?.Running, me?.Repo]);

  return (
    <div className="flex h-full flex-col">
      {failed && (
        <div className="flex items-start gap-3 bg-red-950/70 px-4 py-2 text-sm text-red-100">
          <span className="flex shrink-0 items-center gap-1 font-medium"><CircleX className="h-4 w-4" /> failed to start</span>
          <span className="min-w-0 flex-1 whitespace-pre-wrap font-mono text-xs text-red-200">{failReason || me!.Status}</span>
          <button className="btn shrink-0" onClick={() => run("Retrying agent", AgentService.Retry(name, ""))}>
            Retry
          </button>
        </div>
      )}
      {blocked && tab !== "terminal" && (
        <div className="flex items-center gap-3 bg-yellow-900/60 px-4 py-2 text-sm text-yellow-100">
          <CircleAlert className="h-4 w-4 shrink-0 text-yellow-400" />
          <span className="truncate">{me!.StateReason || "agent is waiting for you"}</span>
          <button className="btn shrink-0 bg-yellow-700 hover:bg-yellow-600" onClick={() => setTab("terminal")}>
            Respond in terminal
          </button>
        </div>
      )}
      <div className="flex items-center gap-1 border-b border-neutral-800 px-2 py-1.5">
        {TABS.map((t) => (
          <button key={t} onClick={() => setTab(t)}
            className={`rounded px-3 py-1.5 text-sm capitalize ${tab === t ? "bg-neutral-700" : "hover:bg-neutral-800"}`}>
            {t}
          </button>
        ))}
        {me && !me.Running && (
          <span className="ml-2 rounded bg-neutral-800 px-2 py-0.5 text-xs text-neutral-400">
            stopped — {me.State || "exited"}
          </span>
        )}
        {pr && (
          <button
            className={`ml-2 flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium ${pr.state === "MERGED" ? "bg-purple-800 text-purple-100" : pr.state === "CLOSED" ? "bg-red-900 text-red-200" : "bg-green-800 text-green-100"} hover:brightness-125`}
            title={`${pr.title} (${pr.state})`}
            onClick={() => AgentService.OpenURL(pr.url).catch(() => {})}
          >
            PR #{pr.number} <ExternalLink className="h-3 w-3" />
          </button>
        )}
        {others.length > 0 && (
          <select
            className="ml-auto rounded bg-neutral-800 px-2 py-1 text-xs"
            value=""
            onChange={(e) => e.target.value && toggleSplit(e.target.value)}
          >
            <option value="">+ split</option>
            {others.map((a) => <option key={a.Name} value={a.Name}>{a.Name}</option>)}
          </select>
        )}
      </div>
      {me?.Prompt && (
        <div className="truncate border-b border-neutral-800 px-4 py-1 text-xs text-neutral-500" title={me.Prompt}>
          <span className="text-neutral-600">task:</span> {me.Prompt}
        </div>
      )}
      <div className="flex min-h-0 flex-1">
        <div className="min-w-0 flex-1">
          {tab === "terminal" && (me?.Running
            ? <TerminalPane container={name} />
            : <div className="flex h-full items-center justify-center text-neutral-500">
                agent is stopped — no live terminal. Use the Output tab, or Retry to relaunch.
              </div>)}
          {tab === "output" && <OutputTab name={name} />}
          {tab === "info" && <InfoTab name={name} />}
          {tab === "diff" && <DiffTab name={name} />}
        </div>
        {panes.map((n) => (
          <div key={n} className="flex min-w-0 flex-1 flex-col border-l border-neutral-800">
            <div className="flex items-center justify-between border-b border-neutral-800 px-2 py-1 text-xs text-neutral-400">
              <span className="truncate">{n.replace(/^agent-/, "")}</span>
              <button className="hover:text-neutral-100" title="Close split" onClick={() => toggleSplit(n)}><X className="h-3.5 w-3.5" /></button>
            </div>
            <div className="min-h-0 flex-1">
              <TerminalPane container={n} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
