import { describe, expect, it } from "vitest";
import { emptyPipeline, newStep, dumpPipeline, parsePipeline } from "./pipeline";

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
