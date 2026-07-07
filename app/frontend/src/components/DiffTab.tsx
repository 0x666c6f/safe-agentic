import { useEffect, useState } from "react";
import { html as diffHtml } from "diff2html";
import type { ColorSchemeType } from "diff2html/lib/types";
import "diff2html/bundles/css/diff2html.min.css";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";
import { useStore } from "../store";
import { errText } from "../types";

const diffFiles = (diff: string): string[] =>
  [...diff.matchAll(/^diff --git a\/(.+?) b\//gm)].map((m) => m[1]);

// diffStat summarizes a unified diff: files touched and added/removed line
// counts (ignoring the +++/--- file headers).
export function diffStat(diff: string): { files: number; additions: number; deletions: number } {
  let additions = 0, deletions = 0;
  for (const line of diff.split("\n")) {
    if (line.startsWith("+++") || line.startsWith("---")) continue;
    if (line.startsWith("+")) additions++;
    else if (line.startsWith("-")) deletions++;
  }
  return { files: diffFiles(diff).length, additions, deletions };
}

export type Checkpoint = { ref: string; desc: string };

// parseCheckpoints turns `git stash list` output into rows. Each line looks
// like `stash@{0}: On main: checkpoint: my label` — we keep the ref for restore
// and strip the branch/checkpoint prefixes for a readable label.
export function parseCheckpoints(raw: string): Checkpoint[] {
  return raw.split("\n")
    .map((l) => l.match(/^(stash@\{\d+\}):\s*(.*)$/))
    .filter((m): m is RegExpMatchArray => m !== null)
    .map((m) => ({ ref: m[1], desc: m[2].replace(/^On [^:]+:\s*/, "").replace(/^checkpoint:\s*/, "") || m[2] }));
}

export function DiffTab({ name }: { name: string }) {
  const toast = useStore((s) => s.toast);
  const [diff, setDiff] = useState("");
  const [checkpoints, setCheckpoints] = useState("");
  const [ref, setRef] = useState("");
  const [sideBySide, setSideBySide] = useState(false);
  // One key is armed at a time: "revert-all", `revert:<path>`, or `restore:<ref>`.
  // Keyed by path/ref, so arming one file's revert never arms another's.
  const [armed, setArmed] = useState<string | null>(null);

  const reload = () => {
    AgentService.Diff(name).then(setDiff).catch((e: unknown) => toast(errText("diff", e)));
    AgentService.CheckpointList(name).then(setCheckpoints).catch(() => setCheckpoints(""));
  };
  useEffect(() => { setArmed(null); reload(); }, [name]);

  const act = (label: string, fn: () => Promise<unknown>) => async () => {
    try { await fn(); toast(`${label}: ok`); reload(); } catch (e) { toast(errText(label, e)); }
  };
  // armAct arms `key` on the first click and fires on the second.
  const armAct = (key: string, label: string, fn: () => Promise<unknown>) => () => {
    if (armed !== key) { setArmed(key); return; }
    setArmed(null);
    act(label, fn)();
  };

  const files = diffFiles(diff);
  const stat = diffStat(diff);
  const checkpointRows = parseCheckpoints(checkpoints);

  return (
    <div className="flex h-full flex-col overflow-y-auto p-4 text-sm">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <button className="btn" onClick={reload}>Refresh</button>
        <button className="btn" onClick={act("stage", () => AgentService.WorkspaceStage(name))}>Stage all</button>
        <button
          className={armed === "revert-all" ? "rounded bg-red-700 px-3 py-1 text-xs text-red-50" : "btn text-red-300 hover:bg-red-900/60"}
          title="Discards all uncommitted changes in the agent's working tree"
          onClick={armAct("revert-all", "revert all", () => AgentService.WorkspaceRevert(name))}>
          {armed === "revert-all" ? "revert all — sure?" : "Revert all"}
        </button>
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
          {files.map((f) => {
            const armKey = `revert:${f}`;
            return (
              <span key={f} className="flex items-center gap-1 rounded bg-neutral-800 px-2 py-0.5 text-xs">
                <span className="max-w-56 truncate" title={f}>{f}</span>
                <button className="text-neutral-400 hover:text-green-400" title="stage this file"
                  onClick={act(`stage ${f}`, () => AgentService.WorkspaceStagePath(name, f))}>＋</button>
                <button className={armed === armKey ? "text-red-400" : "text-neutral-400 hover:text-red-400"}
                  title={armed === armKey ? "click again to revert this file" : "revert this file"}
                  onClick={armAct(armKey, `revert ${f}`, () => AgentService.WorkspaceRevertPath(name, f))}>
                  {armed === armKey ? "↺?" : "↺"}
                </button>
              </span>
            );
          })}
        </div>
      )}
      {checkpointRows.length > 0 && (
        <div className="mb-3 flex flex-col gap-1">
          {checkpointRows.map((c) => {
            const armKey = `restore:${c.ref}`;
            return (
              <div key={c.ref} className="flex items-center gap-2 rounded bg-neutral-900 px-2 py-1 text-xs">
                <span className="font-mono text-neutral-500">{c.ref}</span>
                <span className="min-w-0 flex-1 truncate text-neutral-300" title={c.desc}>{c.desc}</span>
                <button
                  className={armed === armKey ? "rounded bg-red-700 px-2 text-red-50" : "btn"}
                  title="Restore the working tree to this snapshot (discards current changes)"
                  onClick={armAct(armKey, `restore ${c.ref}`, () => AgentService.CheckpointRestore(name, c.ref))}>
                  {armed === armKey ? "sure?" : "Restore"}
                </button>
              </div>
            );
          })}
        </div>
      )}
      {diff.trim() ? (
        <>
          <div className="mb-2 flex items-center gap-3 text-xs text-neutral-400">
            <span>{stat.files} file{stat.files === 1 ? "" : "s"} changed</span>
            <span className="text-green-400">+{stat.additions}</span>
            <span className="text-red-400">−{stat.deletions}</span>
          </div>
          <div className="diff-container overflow-x-auto rounded"
            dangerouslySetInnerHTML={{ __html: diffHtml(diff, { drawFileList: files.length > 1, outputFormat: sideBySide ? "side-by-side" : "line-by-line", colorScheme: "dark" as ColorSchemeType }) }} />
        </>
      ) : <div className="text-neutral-500">no changes</div>}
    </div>
  );
}
