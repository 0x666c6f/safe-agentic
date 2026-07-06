import { useEffect, useMemo, useState } from "react";
import { useStore, statusFor } from "../store";
import { errText } from "../types";
import { StatusDot } from "./StatusDot";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { Service } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/state";
import type { Agent } from "../types";

const STARTER = `name: my-pipeline
steps:
  - name: step-1
    type: claude
    repo: \${repo}
    ssh: true
    reuse_auth: true
    background: true
    prompt: |
      Describe what this step should do.
`;

// Extract ${var} names from a manifest.
const varsOf = (yaml: string): string[] =>
  [...new Set([...yaml.matchAll(/\$\{(\w+)\}/g)].map((m) => m[1]))];

export function PipelinesView() {
  const { toast, agents, needsYou, reviewReady, select, setView } = useStore();
  const [pipelines, setPipelines] = useState<string[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [dirty, setDirty] = useState(false);
  const [vars, setVars] = useState<Record<string, string>>({});
  const [result, setResult] = useState("");
  const [busy, setBusy] = useState(false);
  const [newName, setNewName] = useState<string | null>(null); // non-null = naming a new pipeline

  const reload = () => Service.PipelineList().then((p: string[] | null) => setPipelines(p ?? [])).catch(() => {});
  useEffect(() => { reload(); }, []);

  const openPipeline = async (name: string) => {
    try {
      const c = await Service.PipelineRead(name);
      setSelected(name); setContent(c); setDirty(false); setResult(""); setVars({});
    } catch (e) { toast(errText("read pipeline", e)); }
  };

  const createPipeline = (name: string) => {
    const n = name.trim();
    if (!n) return;
    setSelected(n); setContent(STARTER); setDirty(true); setResult(""); setVars({}); setNewName(null);
  };

  const save = async () => {
    if (!selected) return;
    try { await Service.PipelineSave(selected, content); setDirty(false); reload(); toast(`saved ${selected}`); }
    catch (e) { toast(errText("save pipeline", e)); }
  };

  const del = async () => {
    if (!selected) return;
    try { await Service.PipelineDelete(selected); toast(`deleted ${selected}`); setSelected(null); setContent(""); reload(); }
    catch (e) { toast(errText("delete pipeline", e)); }
  };

  const run = async (dryRun: boolean) => {
    if (!selected) return;
    setBusy(true); setResult("");
    try {
      const out = await AgentService.PipelineRun(selected, vars, dryRun);
      setResult(out || (dryRun ? "dry-run: valid" : "started"));
      if (!dryRun) toast(`pipeline ${selected} started`);
    } catch (e) { setResult(errText(dryRun ? "dry-run" : "run", e)); }
    finally { setBusy(false); }
  };

  const detectedVars = useMemo(() => varsOf(content), [content]);
  const fleets = new Map<string, Agent[]>();
  for (const a of agents) if (a.Fleet) fleets.set(a.Fleet, [...(fleets.get(a.Fleet) ?? []), a]);

  return (
    <div className="flex h-full min-h-0">
      {/* Pipeline list */}
      <div className="flex w-56 shrink-0 flex-col border-r border-neutral-800">
        <div className="flex items-center justify-between px-3 py-2">
          <span className="text-sm font-semibold">Pipelines</span>
          <button className="btn" onClick={() => setNewName("")}>+ New</button>
        </div>
        {newName !== null && (
          <input
            autoFocus
            className="input mx-2 mb-1 text-xs"
            placeholder="name (e.g. reviews/my-review) ↵"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") createPipeline(newName);
              else if (e.key === "Escape") setNewName(null);
            }}
            onBlur={() => newName.trim() ? createPipeline(newName) : setNewName(null)}
          />
        )}
        <div className="min-h-0 flex-1 overflow-y-auto">
          {pipelines.map((p) => (
            <button key={p} onClick={() => openPipeline(p)}
              className={`block w-full truncate px-3 py-1.5 text-left text-sm hover:bg-neutral-800 ${selected === p ? "bg-neutral-800 text-neutral-100" : "text-neutral-300"}`}>
              {p}
            </button>
          ))}
          {pipelines.length === 0 && <div className="px-3 py-4 text-xs text-neutral-600">No saved pipelines.</div>}
        </div>
      </div>

      {/* Editor + run */}
      <div className="flex min-w-0 flex-1 flex-col">
        {selected ? (
          <>
            <div className="flex items-center gap-2 border-b border-neutral-800 px-3 py-2">
              <span className="truncate text-sm font-medium">{selected}{dirty ? " •" : ""}</span>
              <button className="btn ml-auto" disabled={!dirty} onClick={save}>Save</button>
              <button className="btn" disabled={busy} onClick={() => run(true)}>Dry-run</button>
              <button className="btn bg-green-800 hover:bg-green-700" disabled={busy || dirty} title={dirty ? "save first" : ""} onClick={() => run(false)}>▶ Run</button>
              <button className="text-neutral-500 hover:text-red-400" title="delete" onClick={del}>✕</button>
            </div>
            {detectedVars.length > 0 && (
              <div className="flex flex-wrap items-center gap-2 border-b border-neutral-800 px-3 py-2">
                <span className="text-xs text-neutral-500">inputs:</span>
                {detectedVars.map((v) => (
                  <label key={v} className="flex items-center gap-1 text-xs">
                    {v}=
                    <input className="input w-40" value={vars[v] ?? ""}
                      onChange={(e) => setVars((s) => ({ ...s, [v]: e.target.value }))} />
                  </label>
                ))}
              </div>
            )}
            <textarea
              className="min-h-0 flex-1 resize-none bg-neutral-950 p-3 font-mono text-xs text-neutral-200 outline-none"
              spellCheck={false}
              value={content}
              onChange={(e) => { setContent(e.target.value); setDirty(true); }}
            />
            {result && (
              <pre className="max-h-48 overflow-y-auto whitespace-pre-wrap border-t border-neutral-800 bg-neutral-900 p-3 text-xs">{result}</pre>
            )}
          </>
        ) : (
          <div className="flex h-full items-center justify-center text-neutral-500">
            Select a pipeline to edit, or + New to create one.
          </div>
        )}
      </div>

      {/* Running orchestration */}
      {fleets.size > 0 && (
        <div className="w-64 shrink-0 overflow-y-auto border-l border-neutral-800 p-3">
          <div className="mb-2 text-xs font-semibold uppercase text-neutral-500">Running</div>
          {[...fleets.entries()].map(([name, list]) => (
            <div key={name} className="mb-3">
              <div className="mb-1 text-xs text-neutral-400">{name}</div>
              {list.sort((a, b) => a.Hierarchy.localeCompare(b.Hierarchy)).map((a) => (
                <div key={a.Name} className="flex items-center gap-2 py-0.5 text-xs">
                  <StatusDot status={statusFor(a, needsYou, reviewReady)} />
                  <button className="truncate hover:underline" onClick={() => { select(a.Name); setView("agents"); }}>
                    {a.Name.replace(/^agent-/, "")}
                  </button>
                </div>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
