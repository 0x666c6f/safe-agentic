import { create } from "zustand";
import type { Agent, AgentStatus, View } from "./types";

let toastSeq = 0;
const NEEDS = new Set(["needs-auth", "stuck", "blocked"]);
const REVIEW = new Set(["ready-for-review", "ready-for-pr"]);

export function statusFor(
  a: Agent,
  needsYou: Record<string, boolean>,
  reviewReady: Record<string, boolean>,
): AgentStatus {
  if (!a.Running) return a.Finished ? "stopped" : "failed";
  if (needsYou[a.Name] || a.State === "blocked") return "needs-you";
  if (a.Activity === "Working") return "working";
  if (reviewReady[a.Name]) return "review";
  return "idle";
}

interface State {
  agents: Agent[];
  needsYou: Record<string, boolean>;
  reviewReady: Record<string, boolean>;
  selected: string | null;
  split: string | null;
  vmOk: boolean;
  vmError: string;
  toasts: { id: number; text: string }[];
  view: View;
  setAgents: (agents: Agent[]) => void;
  applyEvent: (status: string, container: string) => void;
  select: (name: string | null) => void;
  setSplit: (name: string | null) => void;
  setVM: (ok: boolean, error: string) => void;
  toast: (text: string) => void;
  dismissToast: (id: number) => void;
  setView: (v: View) => void;
}

export const useStore = create<State>((set) => ({
  agents: [], needsYou: {}, reviewReady: {},
  selected: null, split: null, vmOk: true, vmError: "",
  toasts: [], view: "agents",
  setAgents: (agents) => set({ agents }),
  applyEvent: (status, container) =>
    set((s) => {
      if (!container) return {};
      const needsYou = { ...s.needsYou };
      const reviewReady = { ...s.reviewReady };
      if (NEEDS.has(status)) needsYou[container] = true;
      else delete needsYou[container];
      if (REVIEW.has(status)) reviewReady[container] = true;
      else if (status === "info") delete reviewReady[container];
      return { needsYou, reviewReady };
    }),
  select: (selected) => set({ selected }),
  setSplit: (split) => set({ split }),
  setVM: (vmOk, vmError) => set({ vmOk, vmError }),
  toast: (text) => set((s) => ({ toasts: [...s.toasts, { id: ++toastSeq, text }] })),
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  setView: (view) => set({ view }),
}));
