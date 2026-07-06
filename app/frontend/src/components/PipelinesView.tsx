import { useEffect, useMemo, useState } from "react";
import { useStore, statusFor } from "../store";
import { errText, type Agent } from "../types";
import { StatusDot } from "./StatusDot";
import {
  type Pipeline, type Step, type StepType,
  newStep, emptyPipeline, parsePipeline, dumpPipeline, pipelineVars,
} from "../pipeline";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { Service } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/state";

const Toggle = ({ on, set, label }: { on: boolean; set: (v: boolean) => void; label: string }) => (
  <label className="flex cursor-pointer items-center gap-1.5 text-xs text-neutral-400">
    <input type="checkbox" checked={on} onChange={(e) => set(e.target.checked)} /> {label}
  </label>
);

function StepCard({ step, index, total, all, projects, onChange, onRemove, onMove }: {
  step: Step; index: number; total: number; all: Step[]; projects: string[];
  onChange: (s: Step) => void; onRemove: () => void; onMove: (dir: -1 | 1) => void;
}) {
  const up = (patch: Partial<Step>) => onChange({ ...step, ...patch });
  const priorNames = all.slice(0, index).map((s) => s.name).filter(Boolean);
  return (
    <div className="rounded-lg border border-neutral-800 bg-neutral-900/60">
      <div className="flex items-center gap-2 border-b border-neutral-800 px-3 py-2">
        <span className="flex h-5 w-5 items-center justify-center rounded-full bg-neutral-700 text-xs">{index + 1}</span>
        <input
          className="min-w-0 flex-1 bg-transparent text-sm font-medium text-neutral-100 outline-none"
          value={step.name} placeholder="step name"
          onChange={(e) => up({ name: e.target.value })}
        />
        <div className="flex gap-0.5">
          {(["claude", "codex", "shell"] as StepType[]).map((t) => (
            <button key={t} onClick={() => up({ type: t })}
              className={`rounded px-2 py-0.5 text-xs ${step.type === t ? "bg-blue-800 text-white" : "text-neutral-400 hover:bg-neutral-800"}`}>
              {t}
            </button>
          ))}
        </div>
        <button className="px-1 text-neutral-500 hover:text-neutral-200 disabled:opacity-30" disabled={index === 0} onClick={() => onMove(-1)} title="move up">↑</button>
        <button className="px-1 text-neutral-500 hover:text-neutral-200 disabled:opacity-30" disabled={index === total - 1} onClick={() => onMove(1)} title="move down">↓</button>
        <button className="px-1 text-neutral-500 hover:text-red-400" onClick={onRemove} title="remove step">✕</button>
      </div>
      <div className="flex flex-col gap-2 p-3">
        {step.type !== "shell" && (
          <div className="flex items-center gap-2">
            <input
              className="input min-w-0 flex-1 text-xs" placeholder="repo URL or ${repo}"
              value={step.repo} onChange={(e) => up({ repo: e.target.value })}
            />
            <button className="btn shrink-0" title="use ${repo} input" onClick={() => up({ repo: "${repo}" })}>${"{repo}"}</button>
            {projects.length > 0 && (
              <select className="input shrink-0 text-xs" value="" onChange={(e) => e.target.value && up({ repo: e.target.value })}>
                <option value="">saved…</option>
                {projects.map((p) => <option key={p} value={p}>{p.replace(/^.*[/:]/, "")}</option>)}
              </select>
            )}
          </div>
        )}
        <textarea
          className="input min-h-24 w-full font-mono text-xs" placeholder="What should this step do? (supports ${vars})"
          value={step.prompt} onChange={(e) => up({ prompt: e.target.value })}
        />
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
          {step.type !== "shell" && <Toggle on={step.ssh} set={(v) => up({ ssh: v })} label="ssh" />}
          <Toggle on={step.reuseAuth} set={(v) => up({ reuseAuth: v })} label="reuse auth" />
          <Toggle on={step.reuseGhAuth} set={(v) => up({ reuseGhAuth: v })} label="gh auth" />
          <Toggle on={step.autoTrust} set={(v) => up({ autoTrust: v })} label="auto-trust" />
        </div>
        {priorNames.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 text-xs text-neutral-500">
            runs after:
            {priorNames.map((n) => (
              <label key={n} className="flex items-center gap-1">
                <input type="checkbox" checked={step.dependsOn.includes(n)}
                  onChange={(e) => up({ dependsOn: e.target.checked ? [...step.dependsOn, n] : step.dependsOn.filter((d) => d !== n) })} />
                {n}
              </label>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export function PipelinesView() {
  const { toast, agents, needsYou, reviewReady, select, setView } = useStore();
  const [list, setList] = useState<string[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [model, setModel] = useState<Pipeline | null>(null);
  const [raw, setRaw] = useState<string | null>(null); // set when a pipeline can't be form-edited
  const [rawMode, setRawMode] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [vars, setVars] = useState<Record<string, string>>({});
  const [projects, setProjects] = useState<string[]>([]);
  const [result, setResult] = useState("");
  const [busy, setBusy] = useState(false);
  const [naming, setNaming] = useState<string | null>(null);

  const reload = () => Service.PipelineList().then((p: string[] | null) => setList(p ?? [])).catch(() => {});
  useEffect(() => { reload(); Service.Projects().then((p: any[] | null) => setProjects((p ?? []).map((x) => x.url))).catch(() => {}); }, []);

  const open = async (name: string) => {
    try {
      const text = await Service.PipelineRead(name);
      const parsed = parsePipeline(text);
      setSelected(name); setDirty(false); setResult(""); setVars({});
      if (parsed) { setModel(parsed); setRaw(null); setRawMode(false); }
      else { setModel(null); setRaw(text); setRawMode(true); }
    } catch (e) { toast(errText("read pipeline", e)); }
  };

  const create = (name: string) => {
    const n = name.trim(); if (!n) return;
    setSelected(n); setModel(emptyPipeline(n)); setRaw(null); setRawMode(false);
    setDirty(true); setResult(""); setVars({}); setNaming(null);
  };

  const currentYaml = () => rawMode && raw !== null ? raw : model ? dumpPipeline(model) : "";

  const save = async () => {
    if (!selected) return;
    try { await Service.PipelineSave(selected, currentYaml()); setDirty(false); reload(); toast(`saved ${selected}`); }
    catch (e) { toast(errText("save", e)); }
  };
  const del = async () => {
    if (!selected) return;
    try { await Service.PipelineDelete(selected); toast(`deleted ${selected}`); setSelected(null); setModel(null); setRaw(null); reload(); }
    catch (e) { toast(errText("delete", e)); }
  };
  const run = async (dryRun: boolean) => {
    if (!selected) return;
    setBusy(true); setResult("");
    try {
      const out = await AgentService.PipelineRun(selected, vars, dryRun);
      setResult(out || (dryRun ? "✓ dry-run: valid" : "started"));
      if (!dryRun) toast(`pipeline ${selected} started`);
    } catch (e) { setResult(errText(dryRun ? "dry-run" : "run", e)); }
    finally { setBusy(false); }
  };

  const patchStep = (i: number, s: Step) => { if (!model) return; const steps = [...model.steps]; steps[i] = s; setModel({ ...model, steps }); setDirty(true); };
  const addStep = () => { if (!model) return; setModel({ ...model, steps: [...model.steps, newStep()] }); setDirty(true); };
  const removeStep = (i: number) => { if (!model) return; setModel({ ...model, steps: model.steps.filter((_, j) => j !== i) }); setDirty(true); };
  const moveStep = (i: number, dir: -1 | 1) => {
    if (!model) return; const j = i + dir; if (j < 0 || j >= model.steps.length) return;
    const steps = [...model.steps]; [steps[i], steps[j]] = [steps[j], steps[i]]; setModel({ ...model, steps }); setDirty(true);
  };

  const detectedVars = useMemo(() => {
    if (rawMode && raw !== null) return [...new Set([...raw.matchAll(/\$\{(\w+)\}/g)].map((m) => m[1]))];
    return model ? pipelineVars(model) : [];
  }, [model, raw, rawMode]);

  const fleets = new Map<string, Agent[]>();
  for (const a of agents) if (a.Fleet) fleets.set(a.Fleet, [...(fleets.get(a.Fleet) ?? []), a]);

  return (
    <div className="flex h-full min-h-0">
      {/* list */}
      <div className="flex w-52 shrink-0 flex-col border-r border-neutral-800">
        <div className="flex items-center justify-between px-3 py-2">
          <span className="text-sm font-semibold">Pipelines</span>
          <button className="btn" onClick={() => setNaming("")}>+ New</button>
        </div>
        {naming !== null && (
          <input autoFocus className="input mx-2 mb-1 text-xs" placeholder="name (e.g. reviews/x) ↵"
            value={naming} onChange={(e) => setNaming(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") create(naming); else if (e.key === "Escape") setNaming(null); }}
            onBlur={() => naming.trim() ? create(naming) : setNaming(null)} />
        )}
        <div className="min-h-0 flex-1 overflow-y-auto">
          {list.map((p) => (
            <button key={p} onClick={() => open(p)}
              className={`block w-full truncate px-3 py-1.5 text-left text-sm ${selected === p ? "bg-neutral-800 text-neutral-100" : "text-neutral-300 hover:bg-neutral-800/60"}`}>{p}</button>
          ))}
          {list.length === 0 && <div className="px-3 py-4 text-xs text-neutral-600">No pipelines yet.</div>}
        </div>
      </div>

      {/* builder */}
      <div className="flex min-w-0 flex-1 flex-col">
        {!selected ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-neutral-500">
            <div>Build a multi-step pipeline visually.</div>
            <button className="btn bg-green-800 hover:bg-green-700" onClick={() => setNaming("")}>+ New pipeline</button>
          </div>
        ) : (
          <>
            {/* action bar */}
            <div className="flex items-center gap-2 border-b border-neutral-800 px-3 py-2">
              <span className="truncate text-sm font-semibold">{selected}{dirty ? " •" : ""}</span>
              {model && (
                <button className="btn ml-2" onClick={() => { if (rawMode) { const p = parsePipeline(raw!); if (p) { setModel(p); setRawMode(false); } else toast("YAML uses advanced features — keep editing raw"); } else { setRaw(dumpPipeline(model)); setRawMode(true); } }}>
                  {rawMode ? "◀ Form" : "YAML ▶"}
                </button>
              )}
              <button className="btn ml-auto" disabled={!dirty} onClick={save}>Save</button>
              <button className="btn" disabled={busy} onClick={() => run(true)}>Dry-run</button>
              <button className="btn bg-green-800 hover:bg-green-700 disabled:opacity-50" disabled={busy || dirty} title={dirty ? "save first" : ""} onClick={() => run(false)}>▶ Run</button>
              <button className="px-1 text-neutral-500 hover:text-red-400" onClick={del} title="delete">✕</button>
            </div>

            {/* inputs */}
            {detectedVars.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 border-b border-neutral-800 bg-neutral-900/40 px-3 py-2">
                <span className="text-xs text-neutral-500">inputs:</span>
                {detectedVars.map((v) => (
                  <label key={v} className="flex items-center gap-1 text-xs">
                    <span className="text-neutral-400">{v}</span>
                    <input className="input w-44 text-xs" value={vars[v] ?? ""} placeholder="value" onChange={(e) => setVars((s) => ({ ...s, [v]: e.target.value }))} />
                  </label>
                ))}
              </div>
            )}

            {/* body */}
            <div className="min-h-0 flex-1 overflow-y-auto p-4">
              {rawMode || !model ? (
                <textarea className="input h-full min-h-96 w-full font-mono text-xs" spellCheck={false}
                  value={raw ?? ""} onChange={(e) => { setRaw(e.target.value); setDirty(true); }} />
              ) : (
                <div className="mx-auto flex max-w-3xl flex-col gap-3">
                  <label className="flex items-center gap-2 text-sm">
                    <span className="text-neutral-500">Pipeline name</span>
                    <input className="input flex-1" value={model.name} onChange={(e) => { setModel({ ...model, name: e.target.value }); setDirty(true); }} />
                  </label>
                  {model.steps.map((s, i) => (
                    <StepCard key={s.id} step={s} index={i} total={model.steps.length} all={model.steps} projects={projects}
                      onChange={(ns) => patchStep(i, ns)} onRemove={() => removeStep(i)} onMove={(d) => moveStep(i, d)} />
                  ))}
                  <button className="btn self-start" onClick={addStep}>+ Add step</button>
                </div>
              )}
            </div>

            {result && <pre className="max-h-48 overflow-y-auto whitespace-pre-wrap border-t border-neutral-800 bg-neutral-900 p-3 text-xs">{result}</pre>}
          </>
        )}
      </div>

      {/* running */}
      {fleets.size > 0 && (
        <div className="w-56 shrink-0 overflow-y-auto border-l border-neutral-800 p-3">
          <div className="mb-2 text-xs font-semibold uppercase text-neutral-500">Running</div>
          {[...fleets.entries()].map(([name, l]) => (
            <div key={name} className="mb-3">
              <div className="mb-1 text-xs text-neutral-400">{name}</div>
              {l.sort((a, b) => a.Hierarchy.localeCompare(b.Hierarchy)).map((a) => (
                <div key={a.Name} className="flex items-center gap-2 py-0.5 text-xs">
                  <StatusDot status={statusFor(a, needsYou, reviewReady)} />
                  <button className="truncate hover:underline" onClick={() => { select(a.Name); setView("agents"); }}>{a.Name.replace(/^agent-/, "")}</button>
                </div>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
