import { useEffect, useState } from "react";
import { useStore } from "../store";
import { TerminalPane } from "./TerminalPane";
import { OutputTab } from "./OutputTab";
import { InfoTab } from "./InfoTab";
import { DiffTab } from "./DiffTab";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import type { Tab } from "../types";

const TABS: Tab[] = ["terminal", "diff", "output", "info"];

export function Workspace({ name }: { name: string }) {
  const { split, setSplit, agents, tab, setTab } = useStore();
  const me = agents.find((a) => a.Name === name);
  const others = agents.filter((a) => a.Running && a.Name !== name);
  const blocked = me?.Running && me.State === "blocked";
  // Failed = stopped with a non-clean exit (State "exited", not "done").
  const failed = me && !me.Running && me.State === "exited";
  const [failReason, setFailReason] = useState("");

  useEffect(() => {
    if (!failed) { setFailReason(""); return; }
    AgentService.Output(name)
      .then((i: any) => setFailReason((i?.last_output || "").trim().split("\n").slice(-3).join("\n")))
      .catch(() => {});
  }, [name, failed]);

  return (
    <div className="flex h-full flex-col">
      {failed && (
        <div className="flex items-start gap-3 bg-red-950/70 px-4 py-2 text-sm text-red-100">
          <span className="shrink-0 font-medium">✕ failed to start</span>
          <span className="min-w-0 flex-1 whitespace-pre-wrap font-mono text-xs text-red-200">{failReason || me!.Status}</span>
          <button className="btn shrink-0" onClick={() =>
            AgentService.Retry(name, "").then(() => useStore.getState().toast("retried")).catch((e) => useStore.getState().toast(String(e)))}>
            Retry
          </button>
        </div>
      )}
      {blocked && tab !== "terminal" && (
        <div className="flex items-center gap-3 bg-yellow-900/60 px-4 py-2 text-sm text-yellow-100">
          <span className="truncate">🟡 {me!.StateReason || "agent is waiting for you"}</span>
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
        <select
          className="ml-auto rounded bg-neutral-800 px-2 py-1 text-xs"
          value={split ?? ""}
          onChange={(e) => setSplit(e.target.value || null)}
        >
          <option value="">no split</option>
          {others.map((a) => <option key={a.Name} value={a.Name}>{a.Name}</option>)}
        </select>
      </div>
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
        {split && (
          <div className="min-w-0 flex-1 border-l border-neutral-800">
            <TerminalPane container={split} />
          </div>
        )}
      </div>
    </div>
  );
}
