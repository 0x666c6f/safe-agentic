import { useEffect, useMemo, useState } from "react";
import { useStore, statusFor } from "../store";
import { errText, type Agent } from "../types";
import { StatusDot } from "./StatusDot";
import {
  type Pipeline, type Step, type StepType,
  newStep, emptyPipeline, parsePipeline, dumpPipeline, pipelineVars, parsePrUrl,
} from "../pipeline";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { Service } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/state";

const Toggle = ({ on, set, label }: { on: boolean; set: (v: boolean) => void; label: string }) => (
  <label className="flex cursor-pointer items-center gap-1.5 text-xs text-neutral-400">
    <input type="checkbox" checked={on} onChange={(e) => set(e.target.checked)} /> {label}
  </label>
);

function StepCard({ step, projects, canMoveUp, canMoveDown, onChange, onRemove, onMoveStage }: {
  step: Step; projects: string[]; canMoveUp: boolean; canMoveDown: boolean;
  onChange: (s: Step) => void; onRemove: () => void; onMoveStage: (dir: -1 | 1) => void;
}) {
  const up = (patch: Partial<Step>) => onChange({ ...step, ...patch });
  return (
    <div className="flex min-w-72 flex-1 flex-col rounded-lg border border-neutral-800 bg-neutral-900/60">
      <div className="flex items-center gap-2 border-b border-neutral-800 px-3 py-2">
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
        <button className="px-1 text-neutral-500 hover:text-neutral-200 disabled:opacity-30" disabled={!canMoveUp} onClick={() => onMoveStage(-1)} title="move to previous stage">⇞</button>
        <button className="px-1 text-neutral-500 hover:text-neutral-200 disabled:opacity-30" disabled={!canMoveDown} onClick={() => onMoveStage(1)} title="move to next stage">⇟</button>
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
          className="input min-h-20 w-full font-mono text-xs" placeholder="What should this step do? (supports ${vars})"
          value={step.prompt} onChange={(e) => up({ prompt: e.target.value })}
        />
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
          {step.type !== "shell" && <Toggle on={step.ssh} set={(v) => up({ ssh: v })} label="ssh" />}
          <Toggle on={step.reuseAuth} set={(v) => up({ reuseAuth: v })} label="reuse auth" />
          <Toggle on={step.reuseGhAuth} set={(v) => up({ reuseGhAuth: v })} label="gh auth" />
          <Toggle on={step.autoTrust} set={(v) => up({ autoTrust: v })} label="auto-trust" />
        </div>
      </div>
    </div>
  );
}

export function PipelinesView() {
  const { toast, agents, needsYou, reviewReady, select, setView } = useStore();
  const [list, setList] = useState<string[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [model, setModel] = useState<Pipeline | null>(null);
  const [raw, setRaw] = useState<string | null>(null);
  const [rawMode, setRawMode] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [vars, setVars] = useState<Record<string, string>>({});
  const [projects, setProjects] = useState<string[]>([]);
  const [result, setResult] = useState("");
  const [busy, setBusy] = useState(false);
  const [naming, setNaming] = useState<string | null>(null);
  const [prUrl, setPrUrl] = useState("");

  const reload = () => Service.PipelineList().then((p: string[] | null) => setList(p ?? [])).catch(() => {});
  useEffect(() => { reload(); Service.Projects().then((p: any[] | null) => setProjects((p ?? []).map((x) => x.url))).catch(() => {}); }, []);

  const open = async (name: string) => {
    try {
      const text = await Service.PipelineRead(name);
      const parsed = parsePipeline(text);
      setSelected(name); setDirty(false); setResult(""); setVars({}); setPrUrl("");
      if (parsed) { setModel(parsed); setRaw(null); setRawMode(false); }
      else { setModel(null); setRaw(text); setRawMode(true); }
    } catch (e) { toast(errText("read pipeline", e)); }
  };

  const create = (name: string) => {
    const n = name.trim(); if (!n) return;
    setSelected(n); setModel(emptyPipeline(n)); setRaw(null); setRawMode(false);
    setDirty(true); setResult(""); setVars({}); setPrUrl(""); setNaming(null);
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

  const mutate = (fn: (p: Pipeline) => Pipeline) => { if (!model) return; setModel(fn(model)); setDirty(true); };
  const patchStep = (si: number, wi: number, s: Step) =>
    mutate((p) => { const stages = p.stages.map((st) => [...st]); stages[si][wi] = s; return { ...p, stages }; });
  const removeStep = (si: number, wi: number) =>
    mutate((p) => {
      const stages = p.stages.map((st) => [...st]);
      stages[si].splice(wi, 1);
      return { ...p, stages: stages.filter((st) => st.length > 0) };
    });
  const addParallel = (si: number) =>
    mutate((p) => { const stages = p.stages.map((st) => [...st]); stages[si].push(newStep()); return { ...p, stages }; });
  const addStage = (after: number) =>
    mutate((p) => { const stages = p.stages.map((st) => [...st]); stages.splice(after + 1, 0, [newStep()]); return { ...p, stages }; });
  const moveStage = (si: number, wi: number, dir: -1 | 1) =>
    mutate((p) => {
      const stages = p.stages.map((st) => [...st]);
      const [s] = stages[si].splice(wi, 1);
      stages[si + dir].push(s);
      return { ...p, stages: stages.filter((st) => st.length > 0) };
    });

  const detectedVars = useMemo(() => {
    if (rawMode && raw !== null) return [...new Set([...raw.matchAll(/\$\{(\w+)\}/g)].map((m) => m[1]))];
    return model ? pipelineVars(model) : [];
  }, [model, raw, rawMode]);

  // When the pipeline takes both a repo-ish and a pr-ish input, offer a single
  // "PR URL" field that fills both from one pasted GitHub PR link.
  const prFill = useMemo(() => {
    const repoVar = detectedVars.find((v) => /repo/i.test(v));
    const prVar = detectedVars.find((v) => /^pr$|pull|pr_?num|number/i.test(v));
    return repoVar && prVar ? { repoVar, prVar } : null;
  }, [detectedVars]);

  // Match a pipeline step to its spawned container: names end with
  // -<stepName>-<YYYYMMDD>-<HHMMSS>. Returns undefined when the step hasn't run
  // (e.g. an earlier stage failed), so the tree can show it as "not run".
  const stepAgent = (stepName: string): Agent | undefined => {
    const re = new RegExp(`-${stepName.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}-\\d{8}-\\d{6}$`);
    return agents.find((a) => a.Fleet === selected && re.test(a.Name));
  };
  // Fleets that ran but aren't the selected pipeline — still surface them so a
  // background run stays visible even when you're editing another pipeline.
  const otherRuns = new Map<string, Agent[]>();
  for (const a of agents) if (a.Fleet && a.Fleet !== selected) otherRuns.set(a.Fleet, [...(otherRuns.get(a.Fleet) ?? []), a]);

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
            <div>Build a pipeline visually — parallel steps side by side, stages top to bottom.</div>
            <button className="btn bg-green-800 hover:bg-green-700" onClick={() => setNaming("")}>+ New pipeline</button>
          </div>
        ) : (
          <>
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

            {detectedVars.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 border-b border-neutral-800 bg-neutral-900/40 px-3 py-2">
                <span className="text-xs text-neutral-500">inputs:</span>
                {prFill && (
                  <label className="flex items-center gap-1 text-xs" title="Paste a GitHub PR URL to fill both fields">
                    <span className="text-neutral-400">PR URL</span>
                    <input className="input w-72 text-xs" placeholder="https://github.com/org/repo/pull/123"
                      value={prUrl}
                      onChange={(e) => {
                        const val = e.target.value;
                        setPrUrl(val);
                        const parsed = parsePrUrl(val);
                        if (parsed) setVars((s) => ({ ...s, [prFill.repoVar]: parsed.repo, [prFill.prVar]: parsed.pr }));
                      }} />
                    {prUrl && (parsePrUrl(prUrl) ? <span className="text-green-500">✓</span> : <span className="text-neutral-600">…</span>)}
                  </label>
                )}
                {detectedVars.map((v) => (
                  <label key={v} className="flex items-center gap-1 text-xs">
                    <span className="text-neutral-400">{v}</span>
                    <input className="input w-44 text-xs" value={vars[v] ?? ""} placeholder="value" onChange={(e) => setVars((s) => ({ ...s, [v]: e.target.value }))} />
                  </label>
                ))}
              </div>
            )}

            <div className="min-h-0 flex-1 overflow-y-auto p-4">
              {rawMode || !model ? (
                <textarea className="input h-full min-h-96 w-full font-mono text-xs" spellCheck={false}
                  value={raw ?? ""} onChange={(e) => { setRaw(e.target.value); setDirty(true); }} />
              ) : (
                <div className="mx-auto flex max-w-5xl flex-col gap-1">
                  <label className="mb-2 flex items-center gap-2 text-sm">
                    <span className="text-neutral-500">Pipeline name</span>
                    <input className="input flex-1" value={model.name} onChange={(e) => mutate((p) => ({ ...p, name: e.target.value }))} />
                  </label>
                  {model.stages.map((stage, si) => (
                    <div key={si}>
                      <div className="mb-1 flex items-center gap-2">
                        <span className="rounded bg-neutral-800 px-2 py-0.5 text-xs font-medium text-neutral-300">Stage {si + 1}</span>
                        <span className="text-xs text-neutral-600">{stage.length > 1 ? `${stage.length} steps in parallel` : "1 step"}</span>
                        <button className="btn ml-auto" onClick={() => addParallel(si)} title="add a step that runs in parallel within this stage">⫲ + parallel step</button>
                      </div>
                      <div className="flex flex-wrap gap-3">
                        {stage.map((s, wi) => (
                          <StepCard key={s.id} step={s} projects={projects}
                            canMoveUp={si > 0} canMoveDown={si < model.stages.length - 1}
                            onChange={(ns) => patchStep(si, wi, ns)}
                            onRemove={() => removeStep(si, wi)}
                            onMoveStage={(d) => moveStage(si, wi, d)} />
                        ))}
                      </div>
                      <div className="flex flex-col items-center py-1 text-neutral-600">
                        <span>↓</span>
                        <button className="rounded px-2 py-0.5 text-xs text-neutral-500 hover:bg-neutral-800 hover:text-neutral-300"
                          onClick={() => addStage(si)} title="add a stage that runs after this one">+ stage after</button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {result && <pre className="max-h-48 overflow-y-auto whitespace-pre-wrap border-t border-neutral-800 bg-neutral-900 p-3 text-xs">{result}</pre>}
          </>
        )}
      </div>

      {/* run tree — the selected pipeline's structure with live/final status.
          Built from the manifest so it persists after agents stop. */}
      {selected && (model || otherRuns.size > 0) && (
        <div className="w-64 shrink-0 overflow-y-auto border-l border-neutral-800 p-3 text-xs">
          {model && (
            <div className="mb-3">
              <div className="mb-1 flex items-center gap-1 font-semibold text-neutral-300">
                <span>🔄</span><span className="truncate">{selected}</span>
              </div>
              {model.stages.map((stage, si) => {
                const last = si === model.stages.length - 1;
                return (
                  <div key={si} className="flex">
                    <span className="select-none pr-1 font-mono text-neutral-700">{last ? "└─" : "├─"}</span>
                    <div className="min-w-0 flex-1">
                      <div className="text-neutral-500">📦 Stage {si + 1}{stage.length > 1 ? " ∥" : ""}</div>
                      <div className={`ml-1 flex flex-col gap-0.5 border-l pl-2 ${last ? "border-transparent" : "border-neutral-800"}`}>
                        {stage.map((step) => {
                          const a = stepAgent(step.name);
                          return a ? (
                            <button key={step.id} className="flex items-center gap-1.5 py-0.5 text-left hover:underline"
                              onClick={() => { select(a.Name); setView("agents"); }}>
                              <StatusDot status={statusFor(a, needsYou, reviewReady)} />
                              <span className="truncate text-neutral-300">{step.name}</span>
                            </button>
                          ) : (
                            <div key={step.id} className="flex items-center gap-1.5 py-0.5 text-neutral-600">
                              <span className="h-2 w-2 shrink-0 rounded-full border border-neutral-700" />
                              <span className="truncate">{step.name}</span>
                              <span className="text-neutral-700">· not run</span>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
          {[...otherRuns.entries()].map(([name, l]) => (
            <div key={name} className="mb-3">
              <div className="mb-1 flex items-center gap-1 text-neutral-500"><span>🔄</span><span className="truncate">{name}</span></div>
              {l.sort((a, b) => a.Hierarchy.localeCompare(b.Hierarchy)).map((a) => (
                <button key={a.Name} className="flex w-full items-center gap-1.5 py-0.5 text-left hover:underline"
                  onClick={() => { select(a.Name); setView("agents"); }}>
                  <StatusDot status={statusFor(a, needsYou, reviewReady)} />
                  <span className="truncate text-neutral-400">{a.Name.replace(/^agent-/, "")}</span>
                </button>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
