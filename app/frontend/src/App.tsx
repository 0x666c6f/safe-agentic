import { useEffect } from "react";
import { Events } from "@wailsio/runtime";
import { useStore } from "./store";
import { Sidebar } from "./components/Sidebar";
import { Workspace } from "./components/Workspace";
import { SpawnForm } from "./components/SpawnForm";
import { FleetView } from "./components/FleetView";
import { Timeline } from "./components/Timeline";
import { CostView } from "./components/CostView";
import { Palette } from "./components/Palette";
import { VMBanner } from "./components/VMBanner";
import { Toasts } from "./components/Toasts";
import { AgentService } from "../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import type { Agent } from "./types";

const unwrap = (e: any) => (Array.isArray(e?.data) ? e.data[0] : e?.data);

export default function App() {
  const { setAgents, applyEvent, setVM, select, view, selected } = useStore();

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
      Events.On("focus.agent", (e: any) => select(unwrap(e) ?? null)),
    ];
    return () => offs.forEach((off) => off());
  }, []);

  return (
    <div className="flex h-screen flex-col bg-neutral-950 text-neutral-100">
      <VMBanner />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        <main className="min-w-0 flex-1">
          {view === "agents" && (selected
            ? <Workspace key={selected} name={selected} />
            : <div className="p-4 text-neutral-500">Select an agent</div>)}
          {view === "spawn" && <SpawnForm />}
          {view === "fleet" && <FleetView />}
          {view === "timeline" && <Timeline />}
          {view === "cost" && <CostView />}
        </main>
      </div>
      <Palette />
      <Toasts />
    </div>
  );
}
