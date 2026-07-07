import { useEffect, useState } from "react";
import { html as diffHtml } from "diff2html";
import "diff2html/bundles/css/diff2html.min.css";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { useStore } from "../store";
import { errText } from "../types";

const diffFiles = (diff: string): string[] =>
  [...diff.matchAll(/^diff --git a\/(.+?) b\//gm)].map((m) => m[1]);

export function DiffTab({ name }: { name: string }) {
  const toast = useStore((s) => s.toast);
  const [diff, setDiff] = useState("");
  const [checkpoints, setCheckpoints] = useState("");
  const [ref, setRef] = useState("");
  const [sideBySide, setSideBySide] = useState(false);

  const reload = () => {
    AgentService.Diff(name).then(setDiff).catch((e: unknown) => toast(errText("diff", e)));
    AgentService.CheckpointList(name).then(setCheckpoints).catch(() => setCheckpoints(""));
  };
  useEffect(() => { reload(); }, [name]);

  const act = (label: string, fn: () => Promise<unknown>) => async () => {
    try { await fn(); toast(`${label}: ok`); reload(); } catch (e) { toast(errText(label, e)); }
  };
  const files = diffFiles(diff);

  return (
    <div className="flex h-full flex-col overflow-y-auto p-4 text-sm">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <button className="btn" onClick={reload}>Refresh</button>
        <button className="btn" onClick={act("stage", () => AgentService.WorkspaceStage(name))}>Stage all</button>
        <button className="btn" onClick={act("revert", () => AgentService.WorkspaceRevert(name))}>Revert all</button>
        <label className="ml-2 flex items-center gap-1 text-xs">
          <input type="checkbox" checked={sideBySide} onChange={(e) => setSideBySide(e.target.checked)} />
          side-by-side
        </label>
        <span className="mx-2 text-neutral-600">|</span>
        <button className="btn" onClick={act("checkpoint", () => AgentService.CheckpointCreate(name, ""))}>Checkpoint now</button>
        <input className="input w-40" placeholder="ref…" value={ref} onChange={(e) => setRef(e.target.value)} />
        <button className="btn" disabled={!ref} onClick={act("restore", () => AgentService.CheckpointRestore(name, ref))}>Restore</button>
      </div>
      {files.length > 1 && (
        <div className="mb-3 flex flex-wrap gap-1">
          {files.map((f) => (
            <span key={f} className="flex items-center gap-1 rounded bg-neutral-800 px-2 py-0.5 text-xs">
              <span className="max-w-56 truncate" title={f}>{f}</span>
              <button className="text-neutral-400 hover:text-green-400" title="stage this file"
                onClick={act(`stage ${f}`, () => AgentService.WorkspaceStagePath(name, f))}>＋</button>
              <button className="text-neutral-400 hover:text-red-400" title="revert this file"
                onClick={act(`revert ${f}`, () => AgentService.WorkspaceRevertPath(name, f))}>↺</button>
            </span>
          ))}
        </div>
      )}
      {checkpoints && <pre className="mb-3 rounded bg-neutral-900 p-2 text-xs">{checkpoints}</pre>}
      {diff.trim()
        ? <div className="diff-container rounded bg-white text-black"
            dangerouslySetInnerHTML={{ __html: diffHtml(diff, { drawFileList: files.length > 1, outputFormat: sideBySide ? "side-by-side" : "line-by-line" }) }} />
        : <div className="text-neutral-500">no changes</div>}
    </div>
  );
}
