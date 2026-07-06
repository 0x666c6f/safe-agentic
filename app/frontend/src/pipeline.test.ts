import { describe, expect, it } from "vitest";
import { emptyPipeline, newStep, dumpPipeline, parsePipeline, parsePrUrl } from "./pipeline";

describe("parsePrUrl", () => {
  it("splits a PR URL into repo clone URL + number", () => {
    expect(parsePrUrl("https://github.com/org/repo/pull/123"))
      .toEqual({ repo: "https://github.com/org/repo.git", pr: "123" });
  });
  it("tolerates .git, trailing path, and whitespace", () => {
    expect(parsePrUrl("  https://github.com/my-org/my.repo.git/pull/9/files "))
      .toEqual({ repo: "https://github.com/my-org/my.repo.git", pr: "9" });
  });
  it("returns null for non-PR input", () => {
    expect(parsePrUrl("https://github.com/org/repo")).toBeNull();
    expect(parsePrUrl("123")).toBeNull();
  });
});

describe("pipeline stages", () => {
  it("parallel steps share a stage; next stage depends on all of them", () => {
    const p = emptyPipeline("test");
    p.stages[0][0].name = "audit";
    p.stages[0].push({ ...newStep(), name: "tests" });     // parallel with audit
    p.stages.push([{ ...newStep(), name: "consolidate" }]); // sequential after
    const yaml = dumpPipeline(p);

    // stage-0 steps have no depends_on; stage-1 depends on both
    expect(yaml).toContain("depends_on: [audit, tests]");
    const before = yaml.indexOf("depends_on");
    expect(yaml.slice(0, before)).toContain("name: tests"); // both stage-0 steps precede it

    const back = parsePipeline(yaml);
    expect(back).not.toBeNull();
    expect(back!.stages.length).toBe(2);
    expect(back!.stages[0].map((s) => s.name).sort()).toEqual(["audit", "tests"]);
    expect(back!.stages[1][0].name).toBe("consolidate");
  });

  it("partial DAGs fall back to raw mode (null)", () => {
    const yaml = `
name: exotic
steps:
  - name: a
    type: claude
  - name: b
    type: claude
  - name: c
    type: claude
    depends_on: [a]
`;
    expect(parsePipeline(yaml)).toBeNull();
  });

  it("native stages: schema maps to the visual model (parallel review → reconcile)", () => {
    const yaml = `
name: dual-deep-review-reconcile
stages:
  - name: review
    agents:
      - name: claude-review
        type: claude
        repo: \${repo}
        ssh: true
        reuse_auth: true
        background: true
        prompt: |
          Review PR \${pr}.
      - name: codex-review
        type: codex
        repo: \${repo}
        background: true
        prompt: Review PR \${pr}.
  - name: reconcile-fix
    depends_on: [review]
    agents:
      - name: reconcile-fix
        type: codex
        repo: \${repo}
        prompt: Reconcile PR \${pr}.
`;
    const p = parsePipeline(yaml);
    expect(p).not.toBeNull();
    expect(p!.name).toBe("dual-deep-review-reconcile");
    expect(p!.stages.length).toBe(2);
    expect(p!.stages[0].map((s) => s.name).sort()).toEqual(["claude-review", "codex-review"]);
    expect(p!.stages[0].map((s) => s.type).sort()).toEqual(["claude", "codex"]);
    expect(p!.stages[1][0].name).toBe("reconcile-fix");
    // round-trips through the form's steps: serialization
    const back = parsePipeline(dumpPipeline(p!));
    expect(back!.stages.length).toBe(2);
  });

  it("stages with a non-linear dependency or unknown field fall back to raw", () => {
    // stage C depends on A (not the immediately-prior B) → not a linear chain
    const nonLinear = `
name: x
stages:
  - name: a
    agents: [{ name: a1, type: claude }]
  - name: b
    depends_on: [a]
    agents: [{ name: b1, type: claude }]
  - name: c
    depends_on: [a]
    agents: [{ name: c1, type: claude }]
`;
    expect(parsePipeline(nonLinear)).toBeNull();
    // unrepresentable field (memory) → raw, so save can't drop it
    const extra = `
name: y
stages:
  - name: a
    agents:
      - name: a1
        type: claude
        memory: 16g
`;
    expect(parsePipeline(extra)).toBeNull();
  });

  it("built-in style single-step manifests parse", () => {
    const yaml = `
name: review
steps:
  - name: claude-review
    type: claude
    repo: \${repo}
    ssh: true
    prompt: |
      Review PR \${pr}.
`;
    const p = parsePipeline(yaml);
    expect(p).not.toBeNull();
    expect(p!.stages.length).toBe(1);
    expect(p!.stages[0][0].repo).toBe("${repo}");
  });
});
