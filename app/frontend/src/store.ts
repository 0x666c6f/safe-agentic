import { create } from "zustand";
import { errText } from "./types";
import type { Agent, AgentStatus, Tab, View } from "./types";

export type ToastKind = "pending" | "ok" | "error";

// orderAgents returns agents in sidebar display order (fleets, solo, stopped)
// — the canonical order for ⌘1..9 and j/k navigation.
export function orderAgents(agents: Agent[]): Agent[] {
  const fleets: Agent[] = [], solo: Agent[] = [], stopped: Agent[] = [];
  for (const a of agents) {
    if (!a.Running) stopped.push(a);
    else if (a.Fleet) fleets.push(a);
    else solo.push(a);
  }
  return [...fleets, ...solo, ...stopped];
}

let toastSeq = 0;
let spawnSeq = 0;
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
  splits: string[];
  vmOk: boolean;
  vmError: string;
  toasts: { id: number; text: string; kind: ToastKind }[];
  // Optimistic UI: placeholders for spawns in flight (the real container only
  // shows up on the next poll), and names being deleted (greyed until gone).
  pendingSpawns: { id: number; label: string }[];
  deleting: string[];
  view: View;
  tab: Tab;
  setTab: (t: Tab) => void;
  setAgents: (agents: Agent[]) => void;
  applyEvent: (status: string, container: string) => void;
  select: (name: string | null) => void;
  toggleSplit: (name: string) => void;
  setVM: (ok: boolean, error: string) => void;
  toast: (text: string) => void;
  // run wraps a slow action: shows a live "⋯ label" toast while the promise is
  // in flight, then flips it to "✓ label" or the error. Returns the promise so
  // callers can still chain. This is the app's default feedback for any action
  // that isn't instant.
  run: <T>(label: string, p: Promise<T>) => Promise<T>;
  addPendingSpawn: (label: string) => number;
  removePendingSpawn: (id: number) => void;
  markDeleting: (name: string) => void;
  unmarkDeleting: (name: string) => void;
  dismissToast: (id: number) => void;
  setView: (v: View) => void;
}

export const useStore = create<State>()((set) => ({
  agents: [], needsYou: {}, reviewReady: {},
  selected: null, splits: [], vmOk: true, vmError: "",
  toasts: [], pendingSpawns: [], deleting: [], view: "agents", tab: "terminal",
  setTab: (tab) => set({ tab }),
  setAgents: (agents) =>
    set((s) => {
      // Reconcile optimistic state against the fresh poll snapshot:
      // - each newly-appeared container clears one "spawning…" placeholder
      // - drop "deleting" marks for containers that are now gone
      const prev = new Set(s.agents.map((a) => a.Name));
      const appeared = agents.filter((a) => !prev.has(a.Name)).length;
      const pendingSpawns = appeared > 0 ? s.pendingSpawns.slice(appeared) : s.pendingSpawns;
      const present = new Set(agents.map((a) => a.Name));
      const deleting = s.deleting.filter((n) => present.has(n));
      // Split panes only make sense for live terminals — drop stopped agents.
      const splits = s.splits.filter((n) => agents.some((a) => a.Name === n && a.Running));
      return { agents, pendingSpawns, deleting, splits };
    }),
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
  select: (selected) =>
    set((s) => {
      // Default tab follows the agent's state: terminal for running,
      // output for stopped (attach would fail).
      const a = s.agents.find((x) => x.Name === selected);
      // Selecting an agent closes its split pane: the main pane and a split
      // must never attach the same tmux session twice.
      return { selected, splits: s.splits.filter((n) => n !== selected), tab: a && !a.Running ? "output" : "terminal" };
    }),
  toggleSplit: (name) =>
    set((s) => ({
      splits: s.splits.includes(name) ? s.splits.filter((n) => n !== name) : [...s.splits, name],
    })),
  setVM: (vmOk, vmError) => set({ vmOk, vmError }),
  toast: (text) =>
    set((s) => {
      // Dedup identical messages; cap the stack at 5 (drop oldest).
      if (s.toasts.some((t) => t.text === text)) return {};
      const id = ++toastSeq;
      setTimeout(() => useStore.getState().dismissToast(id), 8000);
      const toasts = [...s.toasts, { id, text, kind: "ok" as ToastKind }];
      return { toasts: toasts.slice(-5) };
    }),
  run: async (label, p) => {
    const id = ++toastSeq;
    set((s) => ({ toasts: [...s.toasts, { id, text: label, kind: "pending" as ToastKind }].slice(-5) }));
    const finish = (text: string, kind: ToastKind, ttl: number) => {
      set((s) => ({ toasts: s.toasts.map((t) => (t.id === id ? { ...t, text, kind } : t)) }));
      setTimeout(() => useStore.getState().dismissToast(id), ttl);
    };
    try {
      const out = await p;
      const tail = typeof out === "string" && out.trim()
        ? "\n" + out.trim().split("\n").slice(-2).join("\n") : "";
      finish(`${label}${tail}`, "ok", 6000);
      return out;
    } catch (e) {
      finish(errText(label, e), "error", 10000);
      throw e;
    }
  },
  addPendingSpawn: (label) => {
    const id = ++spawnSeq;
    set((s) => ({ pendingSpawns: [...s.pendingSpawns, { id, label }] }));
    // Safety net: never let a placeholder linger if the container never appears.
    setTimeout(() => useStore.getState().removePendingSpawn(id), 90000);
    return id;
  },
  removePendingSpawn: (id) => set((s) => ({ pendingSpawns: s.pendingSpawns.filter((p) => p.id !== id) })),
  markDeleting: (name) => set((s) => (s.deleting.includes(name) ? {} : { deleting: [...s.deleting, name] })),
  unmarkDeleting: (name) => set((s) => ({ deleting: s.deleting.filter((n) => n !== name) })),
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  setView: (view) => set({ view }),
}));

// Dev-only: expose the store for browser-preview UX testing
// (wails3 dev in a browser has no runtime bridge, so views are driven
// by injecting fixture state).
if (import.meta.env.DEV && typeof window !== "undefined") {
  (window as unknown as Record<string, unknown>).__store = useStore;
}
