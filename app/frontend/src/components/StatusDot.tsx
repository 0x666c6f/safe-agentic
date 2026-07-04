import type { AgentStatus } from "../types";

const COLORS: Record<AgentStatus, string> = {
  working: "bg-green-500", "needs-you": "bg-yellow-400", idle: "bg-gray-400",
  review: "bg-blue-500", failed: "bg-red-500", stopped: "bg-gray-600",
};

export function StatusDot({ status }: { status: AgentStatus }) {
  return <span className={`inline-block w-2.5 h-2.5 rounded-full ${COLORS[status]}`} title={status} />;
}
