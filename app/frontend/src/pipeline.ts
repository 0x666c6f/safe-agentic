// Pipeline form model <-> YAML.
//
// The form models a pipeline as ordered STAGES; steps inside a stage run in
// PARALLEL, stages run sequentially. Serialization: every step in stage N
// gets depends_on = [all step names of stage N-1] — exactly the engine's
// semantics ("stages with no unmet dependencies spawn first").
// Manifests whose depends_on doesn't fit that clean shape (partial DAGs,
// stages:, models fan-out, judge) fall back to raw-YAML editing.
import { load } from "js-yaml";

export type StepType = "claude" | "codex" | "shell";

export interface Step {
  id: number;
  name: string;
  type: StepType;
  repo: string;
  prompt: string;
  ssh: boolean;
  reuseAuth: boolean;
  reuseGhAuth: boolean;
  autoTrust: boolean;
}

export interface Pipeline {
  name: string;
  stages: Step[][]; // stages sequential; steps within a stage parallel
}

let idSeq = 0;
export const newStep = (): Step => ({
  id: ++idSeq, name: `step-${idSeq}`, type: "claude", repo: "${repo}", prompt: "",
  ssh: true, reuseAuth: true, reuseGhAuth: true, autoTrust: true,
});

export const emptyPipeline = (name: string): Pipeline => ({ name, stages: [[newStep()]] });

export function pipelineVars(p: Pipeline): string[] {
  const text = p.stages.flat().map((s) => s.repo + " " + s.prompt).join(" ");
  return [...new Set([...text.matchAll(/\$\{(\w+)\}/g)].map((m) => m[1]))];
}

const sameSet = (a: string[], b: string[]) =>
  a.length === b.length && [...a].sort().join("\0") === [...b].sort().join("\0");

// parsePipeline maps YAML → stage model; null when not form-representable.
export function parsePipeline(text: string): Pipeline | null {
  let doc: any;
  try { doc = load(text); } catch { return null; }
  if (!doc || typeof doc !== "object" || doc.stages) return null;
  const rawSteps = doc.steps;
  if (!Array.isArray(rawSteps)) return null;

  type Parsed = { step: Step; deps: string[] };
  const parsed: Parsed[] = [];
  for (const s of rawSteps) {
    if (!s || typeof s !== "object" || s.models || s.agents || s.judge) return null;
    const type = s.type as StepType;
    if (type !== "claude" && type !== "codex" && type !== "shell") return null;
    parsed.push({
      step: {
        id: ++idSeq,
        name: String(s.name ?? `step-${idSeq}`),
        type,
        repo: String(s.repo ?? ""),
        prompt: String(s.prompt ?? ""),
        ssh: s.ssh !== false,
        reuseAuth: s.reuse_auth !== false,
        reuseGhAuth: s.reuse_gh_auth !== false,
        autoTrust: s.auto_trust === true,
      },
      deps: Array.isArray(s.depends_on) ? s.depends_on.map(String) : [],
    });
  }

  // Group into stages: level 0 = no deps; level N = deps exactly equal to all
  // names at level N-1. Anything else is a partial DAG → raw mode.
  const stages: Step[][] = [];
  let remaining = [...parsed];
  let prevNames: string[] = [];
  while (remaining.length) {
    const here = remaining.filter((p) =>
      stages.length === 0 ? p.deps.length === 0 : sameSet(p.deps, prevNames));
    if (here.length === 0) return null; // partial/exotic deps
    stages.push(here.map((p) => p.step));
    prevNames = here.map((p) => p.step.name);
    remaining = remaining.filter((p) => !here.includes(p));
  }
  return { name: String(doc.name ?? ""), stages };
}

export function dumpPipeline(p: Pipeline): string {
  const out: string[] = [`name: ${p.name}`, `steps:`];
  let prevNames: string[] = [];
  for (const stage of p.stages) {
    for (const s of stage) {
      out.push(`  - name: ${s.name}`);
      out.push(`    type: ${s.type}`);
      if (s.repo.trim()) out.push(`    repo: ${s.repo.trim()}`);
      if (s.ssh) out.push(`    ssh: true`);
      if (s.reuseAuth) out.push(`    reuse_auth: true`);
      if (s.reuseGhAuth) out.push(`    reuse_gh_auth: true`);
      if (s.autoTrust) out.push(`    auto_trust: true`);
      out.push(`    background: true`);
      if (prevNames.length) out.push(`    depends_on: [${prevNames.join(", ")}]`);
      if (s.prompt.trim()) {
        out.push(`    prompt: |`);
        for (const line of s.prompt.replace(/\s+$/, "").split("\n")) out.push(`      ${line}`);
      }
    }
    prevNames = stage.map((s) => s.name);
  }
  return out.join("\n") + "\n";
}
