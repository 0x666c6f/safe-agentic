import { useEffect, useRef, useState } from "react";
import { ExternalLink } from "lucide-react";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";
import type { PRInfo } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc/models";
import { useStore } from "../store";
import { errText } from "../types";

type Info = { status: string; last_output: string; files: string[] | null; commits: string[] | null };

export function OutputTab({ name }: { name: string }) {
  const toast = useStore((s) => s.toast);
  const run = useStore((s) => s.run);
  // The live agent row carries Running (drives auto-refresh) and Repo (needed to
  // look the PR up via gh in its workspace).
  const agent = useStore((s) => s.agents.find((a) => a.Name === name));
  const running = !!agent?.Running;
  const [info, setInfo] = useState<Info | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [steer, setSteer] = useState("");
  const [feedback, setFeedback] = useState("");
  const [armDelete, setArmDelete] = useState(false);
  const [pr, setPr] = useState<PRInfo | null>(null);
  const [review, setReview] = useState("");
  const history = useRef<string[]>([]);
  const histIdx = useRef(-1);

  // refetch pulls the latest output. `silent` skips the loading/error flags so
  // the periodic auto-refresh doesn't flicker the pane or clobber a shown error.
  const refetch = (silent = false) => {
    if (!silent) { setLoading(true); setError(""); }
    return AgentService.Output(name)
      .then((i: any) => setInfo(i))
      .catch((e: unknown) => { if (!silent) setError(errText("load output", e)); })
      .finally(() => { if (!silent) setLoading(false); });
  };
  useEffect(() => { setArmDelete(false); setPr(null); refetch(); }, [name]);

  // Output is otherwise load-once. While the agent runs, refresh it in place
  // every 5s so files/commits/last message stay current without reattaching.
  useEffect(() => {
    if (!running) return;
    const t = setInterval(() => refetch(true), 5000);
    return () => clearInterval(t);
  }, [name, running]);

  const act = (label: string, fn: () => Promise<unknown>) => async () => {
    try { await fn(); toast(`${label}: ok`); refetch(); }
    catch (e) { toast(errText(label, e)); }
  };

  // Stop removes the container (stop + delete), so it arms on first click.
  const del = () => {
    if (!armDelete) { setArmDelete(true); return; }
    setArmDelete(false);
    act("delete", () => AgentService.Stop(name))();
  };

  // The CLI returns nothing from `pr`, so after it runs, look the PR up via gh
  // in the agent's workspace and surface an "Open PR" button.
  const createPR = () =>
    run("Create PR", AgentService.PR(name)).then(
      () => {
        refetch();
        AgentService.PRStatus(name, agent?.Repo ?? "").then((p: any) => { if (p?.url) setPr(p); }).catch(() => {});
      },
      () => { /* run() already surfaced the error toast */ },
    );

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
        <button
          className={armDelete ? "rounded bg-red-700 px-3 py-1 text-xs text-red-50" : "btn text-red-300 hover:bg-red-900/60"}
          title="Removes the container entirely (stop + delete)"
          onClick={del}>
          {armDelete ? "delete — sure?" : "Delete"}
        </button>
        <button className="btn" onClick={createPR}>Create PR</button>
        {pr && (
          <button className="btn bg-green-800 hover:bg-green-700" title={`${pr.title} (${pr.state})`}
            onClick={() => AgentService.OpenURL(pr.url).catch(() => {})}>
            Open PR #{pr.number} <ExternalLink className="h-3 w-3" />
          </button>
        )}
        <button className="btn" onClick={() =>
          run("AI review", AgentService.Review(name)).then((out: any) => setReview(String(out ?? ""))).catch(() => {})
        }>AI Review</button>
        <button className="btn" onClick={() => refetch()}>Reload</button>
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
      {review && (
        <div className="rounded border border-neutral-800">
          <div className="flex items-center justify-between border-b border-neutral-800 px-3 py-1 text-xs text-neutral-400">
            <span>AI review</span>
            <button className="hover:text-neutral-200" onClick={() => setReview("")}>dismiss</button>
          </div>
          <pre className="max-h-80 overflow-y-auto whitespace-pre-wrap p-3 text-xs">{review}</pre>
        </div>
      )}
      <div className="flex gap-2">
        <input className="input flex-1" placeholder="retry feedback (optional)…" value={feedback}
          onChange={(e) => setFeedback(e.target.value)} />
        <button className="btn"
          onClick={act("retry", () => AgentService.Retry(name, feedback))}>Retry</button>
      </div>
    </div>
  );
}
