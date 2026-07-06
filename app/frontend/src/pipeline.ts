// Pipeline form model <-> YAML. The form covers the common single-agent-per
// -step manifest; anything it can't represent (stages, models fan-out, judge)
// falls back to the raw YAML editor via parsePipeline returning null.
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
  dependsOn: string[];
}

export interface Pipeline {
  name: string;
  steps: Step[];
}

let idSeq = 0;
export const newStep = (): Step => ({
  id: ++idSeq, name: `step-${idSeq}`, type: "claude", repo: "${repo}", prompt: "",
  ssh: true, reuseAuth: true, reuseGhAuth: true, autoTrust: true, dependsOn: [],
});

export const emptyPipeline = (name: string): Pipeline => ({ name, steps: [newStep()] });

// Detect ${var} inputs across the whole pipeline.
export function pipelineVars(p: Pipeline): string[] {
  const text = p.steps.map((s) => s.repo + " " + s.prompt).join(" ");
  return [...new Set([...text.matchAll(/\$\{(\w+)\}/g)].map((m) => m[1]))];
}

// parsePipeline maps YAML → form model; returns null when the manifest uses
// features the form doesn't model (stages, models, judge, multiple agents
// per stage), so the caller can drop to raw-YAML editing.
export function parsePipeline(text: string): Pipeline | null {
  let doc: any;
  try { doc = load(text); } catch { return null; }
  if (!doc || typeof doc !== "object") return null;
  if (doc.stages) return null; // staged form — edit as raw
  const rawSteps = doc.steps;
  if (!Array.isArray(rawSteps)) return null;
  const steps: Step[] = [];
  for (const s of rawSteps) {
    if (!s || typeof s !== "object") return null;
    if (s.models || s.agents || s.judge) return null; // not form-modelled
    const type = s.type as StepType;
    if (type !== "claude" && type !== "codex" && type !== "shell") return null;
    steps.push({
      id: ++idSeq,
      name: String(s.name ?? `step-${idSeq}`),
      type,
      repo: String(s.repo ?? ""),
      prompt: String(s.prompt ?? ""),
      ssh: s.ssh !== false,
      reuseAuth: s.reuse_auth !== false,
      reuseGhAuth: s.reuse_gh_auth !== false,
      autoTrust: s.auto_trust === true,
      dependsOn: Array.isArray(s.depends_on) ? s.depends_on.map(String) : [],
    });
  }
  return { name: String(doc.name ?? ""), steps };
}

// dumpPipeline serializes the form model to clean, engine-compatible YAML.
export function dumpPipeline(p: Pipeline): string {
  const out: string[] = [`name: ${p.name}`, `steps:`];
  for (const s of p.steps) {
    out.push(`  - name: ${s.name}`);
    out.push(`    type: ${s.type}`);
    if (s.repo.trim()) out.push(`    repo: ${s.repo.trim()}`);
    if (s.ssh) out.push(`    ssh: true`);
    if (s.reuseAuth) out.push(`    reuse_auth: true`);
    if (s.reuseGhAuth) out.push(`    reuse_gh_auth: true`);
    if (s.autoTrust) out.push(`    auto_trust: true`);
    out.push(`    background: true`);
    if (s.dependsOn.length) out.push(`    depends_on: [${s.dependsOn.join(", ")}]`);
    if (s.prompt.trim()) {
      out.push(`    prompt: |`);
      for (const line of s.prompt.replace(/\s+$/, "").split("\n")) out.push(`      ${line}`);
    }
  }
  return out.join("\n") + "\n";
}
