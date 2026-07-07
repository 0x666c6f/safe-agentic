import { useEffect } from "react";
import { Events } from "@wailsio/runtime";
import { useStore, orderAgents } from "./store";
import { Sidebar } from "./components/Sidebar";
import { Workspace } from "./components/Workspace";
import { SpawnForm } from "./components/SpawnForm";
import { ProjectsView } from "./components/ProjectsView";
import { PipelinesView } from "./components/PipelinesView";
import { Timeline } from "./components/Timeline";
import { CostView } from "./components/CostView";
import { Palette } from "./components/Palette";
import { VMControl } from "./components/VMControl";
import { Toasts } from "./components/Toasts";
import { EmptyState } from "./components/EmptyState";
import { AgentService } from "../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import type { Agent } from "./types";

// Wails v3.0.0-alpha2.112 EventManager.Emit sets Data = data[0] with NO array
// wrapping; the JS runtime passes event.data through raw. Do not "unwrap"
// arrays — []Agent payloads ARE arrays.
const unwrap = (e: any) => e?.data;

const isTyping = (t: EventTarget | null) =>
  t instanceof HTMLElement && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable);

export default function App() {
  const { setAgents, applyEvent, setVM, select, setTab, view, selected, agents, needsYou } = useStore();

  useEffect(() => {
    AgentService.Agents().then((a: Agent[] | null) => setAgents(a ?? []));
    const offs = [
      Events.On("agents.changed", (e: any) => setAgents(unwrap(e) ?? [])),
      Events.On("event.new", (e: any) => {
        const d = unwrap(e);
        applyEvent(d?.status ?? "", d?.event?.payload?.container ?? "");
      }),
      Events.On("vm.status", (e: any) => {
        const d = unwrap(e);
        setVM(!!d?.ok, d?.error ?? "");
      }),
      Events.On("focus.agent", (e: any) => {
        const name = unwrap(e) ?? null;
        select(name);
        if (name) useStore.getState().setView("agents");
      }),
      Events.On("focus.spawn", () => useStore.getState().setView("spawn")),
    ];
    return () => offs.forEach((off) => off());
  }, []);

  // Window title: selected agent + needs-you count (helps ⌘Tab triage).
  useEffect(() => {
    const needs = agents.filter((a) => a.Running && (needsYou[a.Name] || a.State === "blocked")).length;
    document.title = `Safe Agentic${selected ? " — " + selected.replace(/^agent-/, "") : ""}${needs ? ` (${needs} need you)` : ""}`;
  }, [agents, needsYou, selected]);

  // Keyboard: j/k move selection, Enter/⌘1..9 jump, ⌘T/⌘D/⌘O/⌘I tabs.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (isTyping(e.target)) return;
      const s = useStore.getState();
      const order = orderAgents(s.agents);
      if (e.metaKey && e.key >= "1" && e.key <= "9") {
        const a = order[Number(e.key) - 1];
        if (a) { e.preventDefault(); s.select(a.Name); s.setView("agents"); }
        return;
      }
      if (e.metaKey) {
        const tabKeys: Record<string, "terminal" | "diff" | "output" | "info"> =
          { t: "terminal", d: "diff", o: "output", i: "info" };
        const t = tabKeys[e.key];
        if (t && s.selected) { e.preventDefault(); s.setView("agents"); setTab(t); }
        return;
      }
      if (e.key === "j" || e.key === "k") {
        if (!order.length) return;
        const idx = order.findIndex((a) => a.Name === s.selected);
        const next = e.key === "j" ? Math.min(idx + 1, order.length - 1) : Math.max(idx - 1, 0);
        s.select(order[idx === -1 ? 0 : next].Name);
        s.setView("agents");
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [setTab]);

  return (
    <div className="flex h-screen flex-col bg-neutral-950 text-neutral-100">
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        {/* VM bar lives inside the content column so it stops at the sidebar
            edge instead of running underneath the left menu. */}
        <div className="flex min-w-0 flex-1 flex-col">
          <main className="min-h-0 min-w-0 flex-1">
            {view === "agents" && (selected
              ? <Workspace key={selected} name={selected} />
              : <EmptyState />)}
            {view === "spawn" && <SpawnForm />}
            {view === "projects" && <ProjectsView />}
            {view === "fleet" && <PipelinesView />}
            {view === "timeline" && <Timeline />}
            {view === "cost" && <CostView />}
          </main>
          <VMControl />
        </div>
      </div>
      <Palette />
      <Toasts />
    </div>
  );
}
