import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Bell, Bot, CircleDollarSign, Ellipsis, FolderGit2, Workflow } from "lucide-react";
import { useStore, statusFor, orderAgents } from "../store";
import { StatusDot } from "./StatusDot";
import { QuotaBar } from "./QuotaBar";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";
import type { Agent, View } from "../types";

type MenuState = { agent: Agent; x: number; y: number } | null;

function ActionMenu({ menu, close }: { menu: MenuState; close: () => void }) {
  const { run, select, setView, selected, markDeleting, unmarkDeleting } = useStore();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: 0, top: 0 });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [close]);
  useEffect(() => { setConfirmDelete(false); }, [menu]);
  // Clamp against the *measured* menu box (runs before paint, so no flash)
  // instead of guessing its size with hardcoded px — the item count varies.
  useLayoutEffect(() => {
    if (!menu || !ref.current) return;
    const M = 8;
    const r = ref.current.getBoundingClientRect();
    setPos({
      left: Math.max(M, Math.min(menu.x, window.innerWidth - r.width - M)),
      top: Math.max(M, Math.min(menu.y, window.innerHeight - r.height - M)),
    });
  }, [menu, confirmDelete]);
  if (!menu) return null;
  const { agent: a } = menu;
  const act = (label: string, fn: () => Promise<unknown>) => () => {
    close();
    run(label, fn());
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
        ref={ref}
        className="absolute w-56 rounded-md border border-neutral-700 bg-neutral-800 py-1 shadow-xl"
        style={{ left: pos.left, top: pos.top }}
        onClick={(e) => e.stopPropagation()}
      >
        <Item label="Open" onClick={() => { close(); select(a.Name); setView("agents"); }} />
        <Item label="Clone session" onClick={act("Cloning session", () => AgentService.Clone(a.Name))} />
        {!a.Running && <Item label="Retry (same config)" onClick={act("Retrying agent", () => AgentService.Retry(a.Name, ""))} />}
        {a.Running && <Item label="Checkpoint now" onClick={act("Creating checkpoint", () => AgentService.CheckpointCreate(a.Name, ""))} />}
        {a.Running && <Item label="Refresh prefs & restart" onClick={act("Syncing prefs & restarting", () => AgentService.ConfigSync(a.Name, true))} />}
        <Item label="Create PR" onClick={act("Creating PR", () => AgentService.PR(a.Name))} />
        <div className="my-1 border-t border-neutral-700" />
        {confirmDelete
          ? <Item danger label={`Confirm delete ${a.Name.replace(/^agent-/, "")}?`} onClick={() => {
              close();
              const name = a.Name;
              markDeleting(name); // grey the row immediately; poll reconciliation clears it when gone
              run(`Deleting ${name.replace(/^agent-/, "")}`, AgentService.Stop(name))
                .then(() => { if (selected === name) select(null); })
                .catch(() => unmarkDeleting(name));
            }} />
          : <Item danger label="Delete session" onClick={() => setConfirmDelete(true)} />}
      </div>
    </div>
  );
}

const Spinner = ({ tint }: { tint: string }) => (
  <span className={`inline-block h-3 w-3 shrink-0 animate-spin rounded-full border-2 ${tint}`} />
);

// Docker status strings ("Up 3 minutes", "Up About an hour") are verbose for a
// tight sidebar row — condense the uptime to "3m" / "1h". Non-"Up" statuses
// (e.g. "Exited (0)") pass through unchanged.
function humanizeUptime(s: string): string {
  const m = s.match(/^Up\s+(.*)$/i);
  if (!m) return s;
  const t = m[1].toLowerCase().replace(/^(about|less than)\s+/, "");
  const n = parseInt(t, 10) || 1;
  if (t.includes("second")) return `${n}s`;
  if (t.includes("minute")) return `${n}m`;
  if (t.includes("hour")) return `${n}h`;
  if (t.includes("day")) return `${n}d`;
  if (t.includes("week")) return `${n}w`;
  if (t.includes("month")) return `${n}mo`;
  return m[1];
}

function Row({ a, openMenu, index }: { a: Agent; openMenu: (a: Agent, x: number, y: number) => void; index?: number }) {
  const { selected, select, needsYou, reviewReady, setView, deleting } = useStore();
  const st = statusFor(a, needsYou, reviewReady);
  const isSel = selected === a.Name;
  // ⌘1..9 hint: faint index digit matching orderAgents position (first 9 only).
  const digit = index != null && index < 9 ? index + 1 : "";
  if (deleting.includes(a.Name)) {
    return (
      <div className="mx-2 flex items-center gap-2 rounded-md px-2 py-1.5 text-sm opacity-50">
        <Spinner tint="border-neutral-600 border-t-neutral-300" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-neutral-300 line-through">{a.Name.replace(/^agent-/, "")}</span>
          <span className="block text-xs text-neutral-500">deleting…</span>
        </span>
      </div>
    );
  }
  return (
    <div
      className={`group mx-2 flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors ${isSel ? "bg-neutral-800 ring-1 ring-neutral-700" : "hover:bg-neutral-800/60"}`}
      onContextMenu={(e) => { e.preventDefault(); openMenu(a, e.clientX, e.clientY); }}
    >
      <button
        onClick={() => { select(a.Name); setView("agents"); }}
        title={[a.Repo, a.Prompt, a.StateReason || a.Status].filter(Boolean).join(" — ")}
        className="flex min-w-0 flex-1 items-center gap-2 text-left"
      >
        <span className="w-3 shrink-0 text-right text-[10px] tabular-nums text-neutral-600">{digit}</span>
        <StatusDot status={st} />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium text-neutral-100">{a.Name.replace(/^agent-/, "")}</span>
          <span className={`block truncate text-xs ${st === "needs-you" ? "text-yellow-400" : "text-neutral-500"}`}>
            {[a.Repo && a.Repo.replace(/^.*[/:]/, ""), st === "needs-you" ? (a.StateReason || "needs you") : humanizeUptime(a.Status)]
              .filter(Boolean).join(" · ") || a.Type}
          </span>
        </span>
      </button>
      <button
        aria-label={`Actions for ${a.Name.replace(/^agent-/, "")}`}
        className="rounded px-1 text-neutral-500 opacity-0 hover:bg-neutral-700 hover:text-neutral-200 focus:opacity-100 focus-visible:opacity-100 group-hover:opacity-100"
        title="Actions"
        onClick={(e) => { e.stopPropagation(); const r = (e.target as HTMLElement).getBoundingClientRect(); openMenu(a, r.left, r.bottom + 4); }}
      ><Ellipsis className="pointer-events-none h-4 w-4" /></button>
    </div>
  );
}

const NAV: { v: View; icon: typeof Bot; label: string }[] = [
  { v: "agents", icon: Bot, label: "Agents" },
  { v: "projects", icon: FolderGit2, label: "Projects" },
  { v: "fleet", icon: Workflow, label: "Pipelines" },
  { v: "timeline", icon: Bell, label: "Activity" },
  { v: "cost", icon: CircleDollarSign, label: "Cost" },
];

export function Sidebar() {
  const { agents, setView, view, needsYou, pendingSpawns } = useStore();
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
  // ⌘1..9 order (matches the fleets→solo→stopped render order below).
  const orderIdx = new Map(orderAgents(agents).map((a, i) => [a.Name, i]));
  const idxOf = (a: Agent) => orderIdx.get(a.Name);
  const needs = agents.filter((a) => a.Running && (needsYou[a.Name] || a.State === "blocked")).length;
  const SectionLabel = ({ children }: { children: ReactNode }) => (
    <div className="px-4 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-wider text-neutral-600">{children}</div>
  );

  return (
    <aside className="flex h-full w-64 flex-col border-r border-neutral-800 bg-neutral-900 text-neutral-200">
      {/* Header: brand + New agent */}
      <div className="flex items-center gap-2 px-4 pb-2 pt-3">
        <span className="text-sm font-semibold tracking-tight text-neutral-300">Berth</span>
        {needs > 0 && (
          <span className="rounded-full bg-yellow-500/20 px-1.5 text-xs text-yellow-400" title={`${needs} need you`}>{needs}</span>
        )}
        <button
          onClick={() => setView("spawn")}
          className="ml-auto rounded-md bg-green-700 px-2.5 py-1 text-xs font-medium text-white hover:bg-green-600"
        >+ New agent</button>
      </div>

      {/* Agent list — the focus */}
      <div className="min-h-0 flex-1 overflow-y-auto py-1">
        {pendingSpawns.map((p) => (
          <div key={p.id} className="mx-2 flex items-center gap-2 rounded-md px-2 py-1.5 text-sm opacity-80">
            <Spinner tint="border-green-800 border-t-green-400" />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium text-neutral-200">starting…</span>
              <span className="block truncate text-xs text-neutral-500">{p.label}</span>
            </span>
          </div>
        ))}
        {[...fleets.entries()].map(([name, list]) => (
          <div key={name}>
            <SectionLabel><Workflow className="mr-1 inline h-3 w-3" />{name}</SectionLabel>
            {list.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} index={idxOf(a)} />)}
          </div>
        ))}
        {solo.length > 0 && <SectionLabel>Agents</SectionLabel>}
        {solo.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} index={idxOf(a)} />)}
        {stopped.length > 0 && <SectionLabel>Stopped</SectionLabel>}
        {stopped.map((a) => <Row key={a.Name} a={a} openMenu={openMenu} index={idxOf(a)} />)}
        {agents.length === 0 && pendingSpawns.length === 0 && (
          <div className="px-4 py-6 text-center text-xs text-neutral-600">
            No agents yet.<br />Start one with <span className="text-neutral-400">+ New agent</span>.
          </div>
        )}
      </div>

      {/* Quota remaining for Claude + Codex */}
      <QuotaBar />

      {/* Footer nav — secondary views */}
      <nav className="flex items-center justify-around border-t border-neutral-800 px-1 py-1.5">
        {NAV.map(({ v, icon: Icon, label }) => (
          <button
            key={v}
            onClick={() => setView(v)}
            title={label}
            className={`flex flex-1 flex-col items-center gap-0.5 rounded-md py-1 text-[10px] transition-colors ${view === v ? "bg-neutral-800 text-neutral-100" : "text-neutral-500 hover:bg-neutral-800/60 hover:text-neutral-300"}`}
          >
            <Icon className="h-4 w-4" />
            {label}
          </button>
        ))}
      </nav>
      <ActionMenu menu={menu} close={() => setMenu(null)} />
    </aside>
  );
}
