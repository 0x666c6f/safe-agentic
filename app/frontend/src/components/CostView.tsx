import { useEffect, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { useStore } from "../store";
import { errText } from "../types";

export function CostView() {
  const toast = useStore((s) => s.toast);
  const [window, setWindow] = useState("7d");
  const [out, setOut] = useState("");

  useEffect(() => {
    AgentService.CostHistory(window).then(setOut).catch((e: unknown) => { setOut(""); toast(errText("cost", e)); });
  }, [window]);

  return (
    <div className="flex flex-col gap-3 p-6">
      <div className="flex gap-2">
        {["1d", "7d", "30d"].map((w) => (
          <button key={w} className={`btn ${window === w ? "bg-neutral-600" : ""}`} onClick={() => setWindow(w)}>{w}</button>
        ))}
      </div>
      <pre className="whitespace-pre-wrap rounded bg-neutral-900 p-4 text-xs">{out || "no cost data"}</pre>
      <div className="text-xs text-neutral-600">Note: engine pricing table is dated (pkg/cost) — treat numbers as lower bounds.</div>
    </div>
  );
}
