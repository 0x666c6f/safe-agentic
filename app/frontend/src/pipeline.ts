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

// parsePrUrl turns a GitHub PR URL into the (repo clone URL, pr number) pair a
// review pipeline's ${repo}/${pr} inputs want — so you paste one URL instead of
// two fields. Returns null for anything that isn't a PR URL.
export function parsePrUrl(url: string): { repo: string; pr: string } | null {
  const m = url.trim().match(/github\.com[/:]([^/\s]+)\/([^/\s]+?)(?:\.git)?\/pull\/(\d+)/i);
  if (!m) return null;
  return { repo: `https://github.com/${m[1]}/${m[2]}.git`, pr: m[3] };
}

export function pipelineVars(p: Pipeline): string[] {
  const text = p.stages.flat().map((s) => s.repo + " " + s.prompt).join(" ");
  return [...new Set([...text.matchAll(/\$\{(\w+)\}/g)].map((m) => m[1]))];
}

const sameSet = (a: string[], b: string[]) =>
  a.length === b.length && [...a].sort().join("\0") === [...b].sort().join("\0");

// Agent/step fields the Step model round-trips. If a manifest carries anything
// else (memory, notify, models, judge, instructions, …) we bail to raw editing
// so saving from the form can't silently drop it.
const KNOWN_STEP_KEYS = new Set([
  "name", "type", "repo", "prompt", "ssh", "reuse_auth", "reuse_gh_auth",
  "auto_trust", "background", "depends_on",
]);

const toStep = (a: any): Step | null => {
  if (!a || typeof a !== "object") return null;
  for (const k of Object.keys(a)) if (!KNOWN_STEP_KEYS.has(k)) return null;
  const type = a.type as StepType;
  if (type !== "claude" && type !== "codex" && type !== "shell") return null;
  return {
    id: ++idSeq,
    name: String(a.name ?? `step-${idSeq}`),
    type,
    repo: String(a.repo ?? ""),
    prompt: String(a.prompt ?? ""),
    ssh: a.ssh !== false,
    reuseAuth: a.reuse_auth !== false,
    reuseGhAuth: a.reuse_gh_auth !== false,
    autoTrust: a.auto_trust === true,
  };
};

// parseStages handles the engine's native nested schema:
//   stages: [{ name, depends_on?: [stageName], agents: [step] }]
// It maps to the ordered-stage model only when the stage dependencies form a
// linear chain (stage N depends exactly on stage N-1); any other DAG → raw.
function parseStages(doc: any): Pipeline | null {
  const rawStages = doc.stages;
  const stages: Step[][] = [];
  const stageNames: string[] = [];
  for (let i = 0; i < rawStages.length; i++) {
    const st = rawStages[i];
    if (!st || typeof st !== "object" || !Array.isArray(st.agents) || st.matrix) return null;
    const deps = Array.isArray(st.depends_on) ? st.depends_on.map(String)
      : st.depends_on ? [String(st.depends_on)] : [];
    if (!sameSet(deps, i === 0 ? [] : [stageNames[i - 1]])) return null; // non-linear
    const steps: Step[] = [];
    for (const a of st.agents) {
      const step = toStep(a);
      if (!step) return null;
      steps.push(step);
    }
    if (steps.length === 0) return null;
    stages.push(steps);
    stageNames.push(String(st.name ?? `stage-${i}`));
  }
  if (stages.length === 0) return null;
  return { name: String(doc.name ?? ""), stages };
}

// parsePipeline maps YAML → stage model; null when not form-representable.
export function parsePipeline(text: string): Pipeline | null {
  let doc: any;
  try { doc = load(text); } catch { return null; }
  if (!doc || typeof doc !== "object") return null;
  if (Array.isArray(doc.stages)) return parseStages(doc);
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
