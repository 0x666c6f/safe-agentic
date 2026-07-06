import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { orderedStream } from "./orderedStream";

describe("orderedStream", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("reorders out-of-order chunks", () => {
    const out: string[] = [];
    const s = orderedStream((b) => out.push(b));
    s.push("1|a");
    s.push("3|c"); // arrives early
    s.push("2|b");
    expect(out).toEqual(["a", "b", "c"]);
  });

  it("adopts the first seen seq as baseline and drops stale chunks", () => {
    const out: string[] = [];
    const s = orderedStream((b) => out.push(b));
    s.push("5|e"); // pane attached mid-stream
    s.push("4|late"); // before baseline — drop
    s.push("6|f");
    expect(out).toEqual(["e", "f"]);
  });

  it("skips a gap only after the stall timeout, reporting the skip", () => {
    const out: string[] = [];
    let skips = 0;
    const s = orderedStream((b) => out.push(b), 250, () => skips++);
    s.push("1|a");
    s.push("3|c");
    expect(out).toEqual(["a"]); // waiting for 2
    vi.advanceTimersByTime(249);
    expect(out).toEqual(["a"]);
    expect(skips).toBe(0);
    vi.advanceTimersByTime(1);
    expect(out).toEqual(["a", "c"]); // gave up on 2
    expect(skips).toBe(1);
    s.push("4|d");
    expect(out).toEqual(["a", "c", "d"]);
    expect(skips).toBe(1);
  });

  it("passes through unnumbered payloads", () => {
    const out: string[] = [];
    const s = orderedStream((b) => out.push(b));
    s.push("plainb64");
    expect(out).toEqual(["plainb64"]);
  });
});
