import { describe, expect, it, beforeEach } from "vitest";
import { useStore, statusFor } from "./store";
import type { Agent } from "./types";

const agent = (over: Partial<Agent>): Agent => ({
  Name: "agent-x", Type: "claude", Repo: "", Fleet: "", Hierarchy: "",
  Terminal: "tmux", Status: "Up", Running: true, Finished: false,
  Activity: "Idle", State: "", StateReason: "",
  CPU: "", Memory: "", NetIO: "", PIDs: "", SSH: "", Auth: "", GHAuth: "",
  Docker: "", NetworkMode: "", ...over,
});

beforeEach(() => useStore.setState(useStore.getInitialState()));

describe("store", () => {
  it("applies needs-you and clears it on info", () => {
    const s = useStore.getState();
    s.applyEvent("needs-auth", "agent-x");
    expect(useStore.getState().needsYou["agent-x"]).toBe(true);
    useStore.getState().applyEvent("info", "agent-x");
    expect(useStore.getState().needsYou["agent-x"]).toBeFalsy();
  });

  it("statusFor precedence: needs-you > working > review > idle", () => {
    expect(statusFor(agent({ Activity: "Working" }), { "agent-x": true }, {})).toBe("needs-you");
    expect(statusFor(agent({ State: "blocked" }), {}, {})).toBe("needs-you");
    expect(statusFor(agent({ Activity: "Working" }), {}, {})).toBe("working");
    expect(statusFor(agent({}), {}, { "agent-x": true })).toBe("review");
    expect(statusFor(agent({}), {}, {})).toBe("idle");
    expect(statusFor(agent({ Running: false, Finished: false }), {}, {})).toBe("failed");
    expect(statusFor(agent({ Running: false, Finished: true }), {}, {})).toBe("stopped");
  });

  it("toast lifecycle", () => {
    useStore.getState().toast("boom");
    const t = useStore.getState().toasts;
    expect(t).toHaveLength(1);
    useStore.getState().dismissToast(t[0].id);
    expect(useStore.getState().toasts).toHaveLength(0);
  });
});
