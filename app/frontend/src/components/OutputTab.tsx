import { useEffect, useRef, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { useStore } from "../store";
import { errText } from "../types";

type Info = { status: string; last_output: string; files: string[] | null; commits: string[] | null };

export function OutputTab({ name }: { name: string }) {
  const toast = useStore((s) => s.toast);
  const [info, setInfo] = useState<Info | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [steer, setSteer] = useState("");
  const [feedback, setFeedback] = useState("");
  const history = useRef<string[]>([]);
  const histIdx = useRef(-1);

  const reload = () => {
    setLoading(true);
    setError("");
    AgentService.Output(name)
      .then((i: any) => setInfo(i))
      .catch((e: unknown) => setError(errText("load output", e)))
      .finally(() => setLoading(false));
  };
  useEffect(() => { reload(); }, [name]);

  const act = (label: string, fn: () => Promise<unknown>) => async () => {
    try { await fn(); toast(`${label}: ok`); reload(); }
    catch (e) { toast(errText(label, e)); }
  };

  const sendSteer = () => {
    if (!steer.trim()) return;
    history.current = [steer, ...history.current.slice(0, 19)];
    histIdx.current = -1;
    act("steer", () => AgentService.Steer(name, steer).then(() => setSteer("")))();
  };

  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto p-4 text-sm">
      <div className="text-xs text-neutral-500">status: {info?.status ?? "…"}</div>
      {error
        ? <pre className="whitespace-pre-wrap rounded border border-red-900 bg-red-950/40 p-3 text-red-200">{error}</pre>
        : <pre className="whitespace-pre-wrap rounded bg-neutral-900 p-3">{loading ? "loading…" : (info?.last_output || "(no output yet)")}</pre>}
      {(info?.files?.length ?? 0) > 0 && (
        <div>
          <div className="mb-1 text-xs uppercase text-neutral-500">changed files ({info!.files!.length})</div>
          <pre className="max-h-40 overflow-y-auto rounded bg-neutral-900 p-2 text-xs">{info!.files!.join("\n")}</pre>
        </div>
      )}
      {(info?.commits?.length ?? 0) > 0 && (
        <div>
          <div className="mb-1 text-xs uppercase text-neutral-500">commits</div>
          <pre className="max-h-40 overflow-y-auto rounded bg-neutral-900 p-2 text-xs">{info!.commits!.join("\n")}</pre>
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <button className="btn" onClick={act("stop", () => AgentService.Stop(name))}>Stop</button>
        <button className="btn" onClick={act("pr", () => AgentService.PR(name))}>Create PR</button>
        <button className="btn" onClick={act("review", () => AgentService.Review(name))}>AI Review</button>
        <button className="btn" onClick={reload}>Reload</button>
      </div>
      <div className="flex items-end gap-2">
        <textarea
          className="input min-h-16 flex-1 font-mono text-xs"
          placeholder="steer message… (⌘Enter to send, ↑ history)"
          value={steer}
          onChange={(e) => setSteer(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && e.metaKey) { e.preventDefault(); sendSteer(); }
            else if (e.key === "ArrowUp" && !steer) {
              const next = Math.min(histIdx.current + 1, history.current.length - 1);
              if (history.current[next] !== undefined) { histIdx.current = next; setSteer(history.current[next]); }
            } else if (e.key === "Escape") { setSteer(""); histIdx.current = -1; }
          }}
        />
        <button className="btn" disabled={!steer.trim()} onClick={sendSteer}>Steer</button>
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
