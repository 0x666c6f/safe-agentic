import { useEffect, useState, type ReactNode } from "react";
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
  const isSel = selected === a.Name;
  return (
    <div
      className={`group mx-2 flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors ${isSel ? "bg-neutral-800 ring-1 ring-neutral-700" : "hover:bg-neutral-800/60"}`}
      onContextMenu={(e) => { e.preventDefault(); openMenu(a, e.clientX, e.clientY); }}
    >
      <button
        onClick={() => { select(a.Name); setView("agents"); }}
        title={[a.Repo, a.StateReason || a.Status].filter(Boolean).join(" — ")}
        className="flex min-w-0 flex-1 items-center gap-2.5 text-left"
      >
        <StatusDot status={st} />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium text-neutral-100">{a.Name.replace(/^agent-/, "")}</span>
          <span className={`block truncate text-xs ${st === "needs-you" ? "text-yellow-400" : "text-neutral-500"}`}>
            {[a.Repo && a.Repo.replace(/^.*[/:]/, ""), st === "needs-you" ? (a.StateReason || "needs you") : a.Status]
              .filter(Boolean).join(" · ") || a.Type}
          </span>
        </span>
      </button>
      <button
        className="rounded px-1 text-neutral-500 opacity-0 hover:bg-neutral-700 hover:text-neutral-200 group-hover:opacity-100"
        title="Actions"
        onClick={(e) => { e.stopPropagation(); const r = (e.target as HTMLElement).getBoundingClientRect(); openMenu(a, r.left, r.bottom + 4); }}
      >⋯</button>
    </div>
  );
}

const NAV: { v: View; icon: string; label: string }[] = [
  { v: "agents", icon: "◧", label: "Agents" },
  { v: "projects", icon: "🗂", label: "Projects" },
  { v: "fleet", icon: "🔀", label: "Fleet" },
  { v: "timeline", icon: "🔔", label: "Activity" },
  { v: "cost", icon: "＄", label: "Cost" },
];

export function Sidebar() {
  const { agents, setView, view, needsYou } = useStore();
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
  const needs = agents.filter((a) => a.Running && (needsYou[a.Name] || a.State === "blocked")).length;
  const SectionLabel = ({ children }: { children: ReactNode }) => (
    <div className="px-4 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-wider text-neutral-600">{children}</div>
  );

  return (
    <aside className="flex h-full w-64 flex-col border-r border-neutral-800 bg-neutral-900 text-neutral-200">
      {/* Header: brand + New chat */}
      <div className="flex items-center gap-2 px-4 pb-2 pt-3">
        <span className="text-sm font-semibold tracking-tight text-neutral-300">safe-ag</span>
        {needs > 0 && (
          <span className="rounded-full bg-yellow-500/20 px-1.5 text-xs text-yellow-400" title={`${needs} need you`}>{needs}</span>
        )}
        <button
          onClick={() => setView("spawn")}
          className="ml-auto rounded-md bg-green-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-green-600"
        >+ New chat</button>
      </div>

      {/* Agent list — the focus */}
      <div className="min-h-0 flex-1 overflow-y-auto py-1">
        {[...fleets.entries()].map(([name, list]) => (
          <div key={name}>
            <SectionLabel>🔀 {name}</SectionLabel>
            {list.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} />)}
          </div>
        ))}
        {solo.length > 0 && <SectionLabel>Agents</SectionLabel>}
        {solo.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} />)}
        {stopped.length > 0 && <SectionLabel>Stopped</SectionLabel>}
        {stopped.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} />)}
        {agents.length === 0 && (
          <div className="px-4 py-6 text-center text-xs text-neutral-600">
            No agents yet.<br />Start one with <span className="text-neutral-400">+ New chat</span>.
          </div>
        )}
      </div>

      {/* Footer nav — secondary views */}
      <nav className="flex items-center justify-around border-t border-neutral-800 px-1 py-1.5">
        {NAV.map(({ v, icon, label }) => (
          <button
            key={v}
            onClick={() => setView(v)}
            title={label}
            className={`flex flex-1 flex-col items-center gap-0.5 rounded-md py-1 text-[10px] transition-colors ${view === v ? "bg-neutral-800 text-neutral-100" : "text-neutral-500 hover:bg-neutral-800/60 hover:text-neutral-300"}`}
          >
            <span className="text-sm leading-none">{icon}</span>
            {label}
          </button>
        ))}
      </nav>
      <ActionMenu menu={menu} close={() => setMenu(null)} />
    </aside>
  );
}
