export interface Agent {
  Name: string; Type: string; Repo: string; Fleet: string; Hierarchy: string;
  Terminal: string; Status: string; Running: boolean; Finished: boolean;
  Activity: string; State: string; StateReason: string;
  CPU: string; Memory: string; NetIO: string; PIDs: string;
  SSH: string; Auth: string; GHAuth: string; Docker: string; NetworkMode: string;
}
export type AgentStatus = "working" | "needs-you" | "idle" | "review" | "failed" | "stopped";
export type View = "agents" | "fleet" | "timeline" | "cost" | "spawn";

// Human-readable error text: Wails binding rejections are Error objects whose
// message carries the Go error string; bare String() yields just "Error".
export function errText(action: string, e: unknown): string {
  const msg =
    e instanceof Error ? e.message || e.name :
    typeof e === "string" ? e : JSON.stringify(e);
  // Wails RuntimeError rejections serialize the whole error object; show
  // just its human message when the payload is JSON.
  try {
    const j = JSON.parse(msg);
    if (j?.message) return `${action}: ${j.message}`;
  } catch { /* plain string */ }
  return `${action}: ${msg || "unknown error"}`;
}
