import { useEffect, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { useStore } from "../store";

export function OutputTab({ name }: { name: string }) {
  const toast = useStore((s) => s.toast);
  const [info, setInfo] = useState<{ status: string; last_output: string } | null>(null);
  const [steer, setSteer] = useState("");
  const [feedback, setFeedback] = useState("");

  const reload = () =>
    AgentService.Output(name).then((i: any) => setInfo(i)).catch((e: unknown) => toast(String(e)));
  useEffect(() => { reload(); }, [name]);

  const act = (label: string, fn: () => Promise<unknown>) => async () => {
    try { await fn(); toast(`${label}: ok`); reload(); }
    catch (e) { toast(String(e)); }
  };

  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto p-4 text-sm">
      <div className="text-xs text-neutral-500">status: {info?.status ?? "…"}</div>
      <pre className="whitespace-pre-wrap rounded bg-neutral-900 p-3">{info?.last_output ?? "loading…"}</pre>
      <div className="flex flex-wrap gap-2">
        <button className="btn" onClick={act("stop", () => AgentService.Stop(name))}>Stop</button>
        <button className="btn" onClick={act("pr", () => AgentService.PR(name))}>Create PR</button>
        <button className="btn" onClick={act("review", () => AgentService.Review(name))}>AI Review</button>
        <button className="btn" onClick={reload}>Reload</button>
      </div>
      <div className="flex gap-2">
        <input className="input flex-1" placeholder="steer message…" value={steer}
          onChange={(e) => setSteer(e.target.value)} />
        <button className="btn" disabled={!steer}
          onClick={act("steer", () => AgentService.Steer(name, steer).then(() => setSteer("")))}>Steer</button>
      </div>
      <div className="flex gap-2">
        <input className="input flex-1" placeholder="retry feedback (optional)…" value={feedback}
          onChange={(e) => setFeedback(e.target.value)} />
        <button className="btn"
          onClick={act("retry", () => AgentService.Retry(name, feedback))}>Retry</button>
      </div>
    </div>
  );
}
