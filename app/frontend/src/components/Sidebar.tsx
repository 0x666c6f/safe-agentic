import { useEffect, useState } from "react";
import { useStore, statusFor } from "../store";
import { StatusDot } from "./StatusDot";
import { errText } from "../types";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import type { Agent, View } from "../types";

type MenuState = { agent: Agent; x: number; y: number } | null;

function ActionMenu({ menu, close }: { menu: MenuState; close: () => void }) {
  const { toast, select, setView, selected } = useStore();
  const [confirmDelete, setConfirmDelete] = useState(false);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [close]);
  useEffect(() => { setConfirmDelete(false); }, [menu]);
  if (!menu) return null;
  const { agent: a } = menu;
  const act = (label: string, fn: () => Promise<unknown>) => () => {
    close();
    fn().then((out) => toast(typeof out === "string" && out.trim() ? `${label}:\n${out.trim().split("\n").slice(-2).join("\n")}` : `${label}: ok`))
      .catch((e) => toast(errText(label, e)));
  };
  const Item = ({ label, onClick, danger }: { label: string; onClick: () => void; danger?: boolean }) => (
    <button onClick={onClick}
      className={`block w-full px-3 py-1.5 text-left text-sm hover:bg-neutral-700 ${danger ? "text-red-400" : ""}`}>
      {label}
    </button>
  );
  return (
    <div className="fixed inset-0 z-50" onClick={close} onContextMenu={(e) => { e.preventDefault(); close(); }}>
      <div
        className="absolute w-56 rounded-md border border-neutral-700 bg-neutral-800 py-1 shadow-xl"
        style={{ left: Math.min(menu.x, window.innerWidth - 230), top: Math.min(menu.y, window.innerHeight - 260) }}
        onClick={(e) => e.stopPropagation()}
      >
        <Item label="Open" onClick={() => { close(); select(a.Name); setView("agents"); }} />
        <Item label="Clone session" onClick={act("clone", () => AgentService.Clone(a.Name))} />
        {!a.Running && <Item label="Retry (same config)" onClick={act("retry", () => AgentService.Retry(a.Name, ""))} />}
        {a.Running && <Item label="Checkpoint now" onClick={act("checkpoint", () => AgentService.CheckpointCreate(a.Name, ""))} />}
        {a.Running && <Item label="Refresh prefs & restart" onClick={act("config sync", () => AgentService.ConfigSync(a.Name, true))} />}
        <Item label="Create PR" onClick={act("pr", () => AgentService.PR(a.Name))} />
        <div className="my-1 border-t border-neutral-700" />
        {confirmDelete
          ? <Item danger label={`Confirm delete ${a.Name.replace(/^agent-/, "")}?`} onClick={() => {
              close();
              AgentService.Stop(a.Name)
                .then(() => { toast(`deleted ${a.Name}`); if (selected === a.Name) select(null); })
                .catch((e) => toast(errText("delete", e)));
            }} />
          : <Item danger label="Delete session" onClick={() => setConfirmDelete(true)} />}
      </div>
    </div>
  );
}

function Row({ a, openMenu }: { a: Agent; openMenu: (a: Agent, x: number, y: number) => void }) {
  const { selected, select, needsYou, reviewReady, setView } = useStore();
  const st = statusFor(a, needsYou, reviewReady);
  return (
    <div
      className={`group flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-neutral-800 ${selected === a.Name ? "bg-neutral-800" : ""}`}
      onContextMenu={(e) => { e.preventDefault(); openMenu(a, e.clientX, e.clientY); }}
    >
      <button
        onClick={() => { select(a.Name); setView("agents"); }}
        title={[a.Repo, a.StateReason || a.Status].filter(Boolean).join(" — ")}
        className="min-w-0 flex-1 text-left"
      >
        <span className="flex items-center gap-2">
          <StatusDot status={st} />
          <span className="truncate">{a.Name.replace(/^agent-/, "")}</span>
        </span>
        <span className={`block truncate pl-4 text-xs ${st === "needs-you" ? "text-yellow-400" : "text-neutral-500"}`}>
          {[a.Repo, st === "needs-you" ? (a.StateReason || "needs you") : a.Status]
            .filter(Boolean).join(" · ")}
        </span>
      </button>
      <span className="text-xs text-neutral-500 group-hover:hidden">{a.Type}</span>
      <button
        className="hidden rounded px-1 text-neutral-400 hover:bg-neutral-700 group-hover:block"
        title="Actions"
        onClick={(e) => { e.stopPropagation(); const r = (e.target as HTMLElement).getBoundingClientRect(); openMenu(a, r.left, r.bottom + 4); }}
      >⋯</button>
    </div>
  );
}

export function Sidebar() {
  const { agents, setView, view } = useStore();
  const [menu, setMenu] = useState<MenuState>(null);
  const openMenu = (agent: Agent, x: number, y: number) => setMenu({ agent, x, y });

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
            {list.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} />)}
          </div>
        ))}
        {solo.length > 0 && <div className="px-3 pt-2 text-xs uppercase text-neutral-500">agents</div>}
        {solo.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} />)}
        {stopped.length > 0 && <div className="px-3 pt-2 text-xs uppercase text-neutral-500">stopped</div>}
        {stopped.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} />)}
      </div>
      <ActionMenu menu={menu} close={() => setMenu(null)} />
    </aside>
  );
}
