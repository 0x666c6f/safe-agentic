import { useEffect, useState } from "react";
import { Command } from "cmdk";
import { useStore, statusFor } from "../store";
import { errText } from "../types";
import { StatusDot } from "./StatusDot";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";

export function Palette() {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [confirmStopAll, setConfirmStopAll] = useState(false);
  const { agents, needsYou, reviewReady, select, setView, setTab, selected, toast } = useStore();

  const close = () => { setOpen(false); setSearch(""); setConfirmStopAll(false); };
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "k" && e.metaKey) { e.preventDefault(); setOpen((o) => !o); setSearch(""); setConfirmStopAll(false); }
      if (e.key === "Escape") close();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  if (!open) return null;
  const go = (fn: () => void) => () => { fn(); close(); };
  // run labels the toast with the action itself, so a failure reads e.g.
  // "stop foo: <error>" instead of a generic "action: <error>".
  const run = (label: string, p: Promise<unknown>) =>
    p.then(() => toast(`${label}: ok`)).catch((e: unknown) => toast(errText(label, e)));

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
    ["Start VM", () => run("vm start", AgentService.VMStart())],
  ];

  const running = agents.filter((a) => a.Running);
  // "Stop all running" removes every running container, so require a second
  // Enter: the first arms it (relabels), the second fires. Any filter change
  // or close resets the arm (see setSearch / close).
  const stopAll = () => {
    if (!confirmStopAll) { setConfirmStopAll(true); return; }
    running.forEach((a) => run(`stop ${a.Name}`, AgentService.Stop(a.Name)));
    close();
  };

  return (
    <div className="fixed inset-0 z-40 flex items-start justify-center bg-black/50 pt-32" onClick={close}>
      <div onClick={(e) => e.stopPropagation()}>
        <Command className="w-[560px] rounded-lg border border-neutral-700 bg-neutral-900 p-2 shadow-2xl">
          <Command.Input autoFocus placeholder="agent or action…" className="input mb-2 w-full"
            value={search} onValueChange={(v) => { setSearch(v); setConfirmStopAll(false); }} />
          <Command.List className="max-h-80 overflow-y-auto text-sm">
            <Command.Empty className="p-3 text-neutral-500">nothing</Command.Empty>
            {agents.map((a) => {
              const status = statusFor(a, needsYou, reviewReady);
              return (
                <Command.Item key={a.Name} value={`${a.Name} ${status}`}
                  onSelect={go(() => { select(a.Name); setView("agents"); })}
                  className="flex cursor-pointer items-center gap-2 rounded px-3 py-2 data-[selected=true]:bg-neutral-700">
                  <StatusDot status={status} />
                  <span className="min-w-0 flex-1 truncate">{a.Name.replace(/^agent-/, "")}</span>
                  {!a.Running && <span className="shrink-0 text-xs text-neutral-500">{status}</span>}
                </Command.Item>
              );
            })}
            {running.length > 0 && (
              <Command.Item value="stop all running" onSelect={stopAll} keywords={["stop", "all"]}
                className={`cursor-pointer rounded px-3 py-2 data-[selected=true]:bg-neutral-700 ${confirmStopAll ? "text-red-300" : ""}`}>
                {confirmStopAll ? `Press Enter again to confirm — stop ${running.length} running` : `Stop all running (${running.length})`}
              </Command.Item>
            )}
            {actions.map(([label, fn]) => (
              <Command.Item key={label} value={label} onSelect={go(fn)}
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
