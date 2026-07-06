import { useEffect, useState } from "react";
import { Command } from "cmdk";
import { useStore } from "../store";
import { errText } from "../types";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";

export function Palette() {
  const [open, setOpen] = useState(false);
  const { agents, select, setView, setTab, selected, toast } = useStore();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "k" && e.metaKey) { e.preventDefault(); setOpen((o) => !o); }
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  if (!open) return null;
  const go = (fn: () => void) => () => { fn(); setOpen(false); };
  const run = (label: string, p: Promise<unknown>) =>
    p.then(() => toast(`${label}: ok`)).catch((e: unknown) => toast(errText("action", e)));

  const sel = selected?.replace(/^agent-/, "");
  const selActions: [string, () => void][] = selected ? [
    [`Terminal: ${sel}`, () => { setView("agents"); setTab("terminal"); }],
    [`Diff: ${sel}`, () => { setView("agents"); setTab("diff"); }],
    [`Steer: ${sel}`, () => { setView("agents"); setTab("output"); }],
    [`Clone: ${sel}`, () => run(`clone ${sel}`, AgentService.Clone(selected))],
    [`Stop: ${sel}`, () => run(`stop ${sel}`, AgentService.Stop(selected))],
  ] : [];
  const actions: [string, () => void][] = [
    ...selActions,
    ["Spawn agent", () => setView("spawn")],
    ["Pipelines", () => setView("fleet")],
    ["Timeline", () => setView("timeline")],
    ["Cost", () => setView("cost")],
    ["Stop all running", () => agents.filter((a) => a.Running)
      .forEach((a) => run(`stop ${a.Name}`, AgentService.Stop(a.Name)))],
    ["Start VM", () => run("vm start", AgentService.VMStart())],
  ];

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/50 pt-32" onClick={() => setOpen(false)}>
      <div onClick={(e) => e.stopPropagation()}>
        <Command className="w-[560px] rounded-lg border border-neutral-700 bg-neutral-900 p-2 shadow-2xl">
          <Command.Input autoFocus placeholder="agent or action…" className="input mb-2 w-full" />
          <Command.List className="max-h-80 overflow-y-auto text-sm">
            <Command.Empty className="p-3 text-neutral-500">nothing</Command.Empty>
            {agents.filter((a) => a.Running).map((a) => (
              <Command.Item key={a.Name} onSelect={go(() => { select(a.Name); setView("agents"); })}
                className="cursor-pointer rounded px-3 py-2 data-[selected=true]:bg-neutral-700">
                {a.Name}
              </Command.Item>
            ))}
            {actions.map(([label, fn]) => (
              <Command.Item key={label} onSelect={go(fn)}
                className="cursor-pointer rounded px-3 py-2 data-[selected=true]:bg-neutral-700">
                {label}
              </Command.Item>
            ))}
          </Command.List>
        </Command>
      </div>
    </div>
  );
}
