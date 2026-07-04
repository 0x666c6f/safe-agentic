import { useStore, statusFor } from "../store";
import { StatusDot } from "./StatusDot";
import type { Agent, View } from "../types";

function Row({ a }: { a: Agent }) {
  const { selected, select, needsYou, reviewReady, setView } = useStore();
  const st = statusFor(a, needsYou, reviewReady);
  return (
    <button
      onClick={() => { select(a.Name); setView("agents"); }}
      className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-neutral-800 ${selected === a.Name ? "bg-neutral-800" : ""}`}
    >
      <StatusDot status={st} />
      <span className="truncate">{a.Name.replace(/^agent-/, "")}</span>
      <span className="ml-auto text-xs text-neutral-500">{a.Type}</span>
    </button>
  );
}

export function Sidebar() {
  const { agents, setView, view } = useStore();
  const fleets = new Map<string, Agent[]>();
  const solo: Agent[] = [];
  const stopped: Agent[] = [];
  for (const a of agents) {
    if (!a.Running) stopped.push(a);
    else if (a.Fleet) fleets.set(a.Fleet, [...(fleets.get(a.Fleet) ?? []), a]);
    else solo.push(a);
  }
  const NavBtn = ({ v, label }: { v: View; label: string }) => (
    <button onClick={() => setView(v)}
      className={`rounded px-2 py-1 text-xs ${view === v ? "bg-neutral-700" : "hover:bg-neutral-800"}`}>
      {label}
    </button>
  );
  return (
    <aside className="flex h-full w-64 flex-col border-r border-neutral-800 bg-neutral-900 text-neutral-200">
      <div className="flex flex-wrap gap-1 p-2">
        <NavBtn v="agents" label="Agents" /><NavBtn v="fleet" label="Fleet" />
        <NavBtn v="timeline" label="Timeline" /><NavBtn v="cost" label="Cost" />
        <NavBtn v="spawn" label="+ Spawn" />
      </div>
      <div className="flex-1 overflow-y-auto">
        {[...fleets.entries()].map(([name, list]) => (
          <div key={name}>
            <div className="px-3 pt-2 text-xs uppercase text-neutral-500">{name}</div>
            {list.map((a) => <Row key={a.Name} a={a} />)}
          </div>
        ))}
        {solo.length > 0 && <div className="px-3 pt-2 text-xs uppercase text-neutral-500">agents</div>}
        {solo.map((a) => <Row key={a.Name} a={a} />)}
        {stopped.length > 0 && <div className="px-3 pt-2 text-xs uppercase text-neutral-500">stopped</div>}
        {stopped.map((a) => <Row key={a.Name} a={a} />)}
      </div>
    </aside>
  );
}
