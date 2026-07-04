export interface Agent {
  Name: string; Type: string; Repo: string; Fleet: string; Hierarchy: string;
  Terminal: string; Status: string; Running: boolean; Finished: boolean;
  Activity: string; State: string; StateReason: string;
  CPU: string; Memory: string; NetIO: string; PIDs: string;
  SSH: string; Auth: string; GHAuth: string; Docker: string; NetworkMode: string;
}
export type AgentStatus = "working" | "needs-you" | "idle" | "review" | "failed" | "stopped";
export type View = "agents" | "fleet" | "timeline" | "cost" | "spawn";
