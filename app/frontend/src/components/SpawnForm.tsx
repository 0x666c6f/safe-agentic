import { useEffect, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/berth/app/internal/svc";
import { TriangleAlert, X } from "lucide-react";
import { useStore } from "../store";
import { errText } from "../types";
import { recordRepoUse, topRepos, forgetRepo } from "../repoHistory";

const shortRepo = (u: string) =>
  u.replace(/^(git@github\.com:|https:\/\/github\.com\/)/, "").replace(/\.git$/, "");

export function SpawnForm() {
  const { run, setView, addPendingSpawn, removePendingSpawn } = useStore();
  const [busy, setBusy] = useState<"" | "spawn" | "dry">("");
  const [req, setReq] = useState({
    Agent: "claude", Name: "", Repo: "", Prompt: "", Template: "", Network: "",
    Memory: "", CPUs: "", MaxCost: "", WorktreeDir: "", Instructions: "", AWSProfile: "",
    SSH: true, ReuseAuth: false, Worktree: false, DryRun: false,
  });
  const [templates, setTemplates] = useState<string[]>([]);
  const [preview, setPreview] = useState("");
  const [savedRepos, setSavedRepos] = useState<string[]>([]);
  const refreshRepos = () => topRepos(6).then(setSavedRepos).catch(() => {});
  useEffect(() => { refreshRepos(); }, []);

  useEffect(() => {
    AgentService.TemplateList()
      .then((out: string) => setTemplates(
        out.split("\n")
          .map((l) => l.trim().split(/\s+/)[0])
          .filter((t) => t && !/^(NAME|TEMPLATE)$/i.test(t) && !/^[-─═=]+$/.test(t))))
      .catch(() => setTemplates([]));
  }, []);

  const set = (k: string, v: unknown) => setReq((r) => ({ ...r, [k]: v }));
  const submit = async (dryRun: boolean) => {
    if (dryRun) {
      setBusy("dry");
      try { setPreview(await AgentService.Spawn({ ...req, DryRun: true } as any)); }
      catch (e) { setPreview(errText("dry-run", e)); }
      finally { setBusy(""); }
      return;
    }
    // Optimistic: show a "starting…" row and jump to the list right away — the
    // real container only lands on the next poll, so this bridges the gap.
    const label = `${req.Agent}${req.Repo ? " · " + shortRepo(req.Repo) : req.WorktreeDir ? " · " + (req.WorktreeDir.split("/").pop() || "worktree") : ""}`;
    const pid = addPendingSpawn(label);
    setView("agents");
    try {
      await run(`Spawning ${label}`, AgentService.Spawn({ ...req, DryRun: false } as any));
      if (req.Repo) { recordRepoUse(req.Repo); }
      setTimeout(() => AgentService.Refresh(), 1500); // nudge the poller to swap in the real row
    } catch { removePendingSpawn(pid); }
  };

  const Check = ({ k, label, title }: { k: "SSH" | "ReuseAuth" | "Worktree"; label: string; title?: string }) => (
    <label className="flex items-center gap-2 text-sm" title={title}>
      <input type="checkbox" checked={req[k]} onChange={(e) => set(k, e.target.checked)} /> {label}
    </label>
  );

  // Non-blocking format hints — spawn stays enabled even when these look off.
  const memOk = !req.Memory.trim() || /^\d+(\.\d+)?\s*(b|kb|mb|gb|k|m|g)?$/i.test(req.Memory.trim());
  const cpuOk = !req.CPUs.trim() || /^\d+(\.\d+)?$/.test(req.CPUs.trim());
  const canSpawn = !busy && !(req.Worktree && !req.WorktreeDir);

  return (
    <div className="flex max-w-2xl flex-col gap-3 p-6">
      <h2 className="text-lg">Spawn agent</h2>
      <div className="flex gap-2">
        {["claude", "codex", "shell"].map((t) => (
          <button key={t} onClick={() => set("Agent", t)}
            className={req.Agent === t
              ? "rounded bg-blue-800 px-3 py-1 text-xs text-white ring-1 ring-blue-400"
              : "btn"}>{t}</button>
        ))}
      </div>
      <input className="input" placeholder="name (optional — auto-generated if empty)" value={req.Name}
        onChange={(e) => set("Name", e.target.value)} />
      {!req.Worktree && (
        <input className="input" placeholder="repo URL (optional — empty gives a blank workspace)" value={req.Repo}
          onChange={(e) => set("Repo", e.target.value)} />
      )}
      {req.Worktree && (
        <div className="flex items-center gap-2">
          <button className="btn shrink-0" onClick={() =>
            AgentService.PickFolder().then((p: string) => p && set("WorktreeDir", p)).catch(() => {})
          }>Choose local repo…</button>
          <span className={`min-w-0 flex-1 truncate text-xs ${req.WorktreeDir ? "text-neutral-300" : "text-yellow-500"}`}
            title={req.WorktreeDir}>
            {req.WorktreeDir || "worktree mode checks out a git worktree of a LOCAL repo — pick its folder"}
          </span>
        </div>
      )}
      {req.Repo.trim() && !/[/:.]/.test(req.Repo) && (
        <div className="text-xs text-yellow-500">
          <TriangleAlert className="mr-1 inline h-3.5 w-3.5" />"{req.Repo}" doesn't look like a repo URL — the agent will refuse to clone it. Leave empty for a blank workspace.
        </div>
      )}
      {savedRepos.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {savedRepos.map((r) => (
            <span key={r} className="flex items-center gap-1 rounded bg-neutral-800 px-2 py-0.5 text-xs">
              <button className={req.Repo === r ? "text-blue-300" : "hover:text-blue-300"}
                title={r} onClick={() => set("Repo", r)}>
                {r.replace(/^(git@github\.com:|https:\/\/github\.com\/)/, "").replace(/\.git$/, "")}
              </button>
              <button className="text-neutral-500 hover:text-red-400" title="forget"
                onClick={() => forgetRepo(r).then(refreshRepos)}><X className="h-3 w-3" /></button>
            </span>
          ))}
        </div>
      )}
      <textarea className="input min-h-24" placeholder="prompt (optional — ⌘Enter to spawn)" value={req.Prompt}
        onChange={(e) => set("Prompt", e.target.value)}
        onKeyDown={(e) => { if (e.key === "Enter" && e.metaKey && canSpawn) { e.preventDefault(); submit(false); } }} />
      <textarea className="input min-h-12" value={req.Instructions}
        placeholder="standing instructions (optional — always-on context, e.g. focus areas or constraints)"
        onChange={(e) => set("Instructions", e.target.value)} />
      <select className="input" value={req.Template} onChange={(e) => set("Template", e.target.value)}>
        <option value="">no template</option>
        {templates.map((t) => <option key={t} value={t}>{t}</option>)}
      </select>
      <div className="flex gap-4">
        <Check k="SSH" label="SSH agent forwarding" title="SSH agent forwarding — lets the agent clone/push private repos via git@" />
        <Check k="ReuseAuth" label="Reuse shared auth" title="Reuse shared auth volume — persistent login shared across agents" />
        <label className="flex items-center gap-2 text-sm" title="Local worktree — work on a git worktree of a local checkout instead of cloning a repo URL (needs: berth setup --enable-worktrees)">
          <input type="checkbox" checked={req.Worktree}
            onChange={(e) => setReq((r) => ({ ...r, Worktree: e.target.checked, ...(e.target.checked ? { Repo: "" } : { WorktreeDir: "" }) }))} /> Local worktree
        </label>
      </div>
      <div className="flex gap-2">
        <input className="input flex-1" placeholder="network (blank = dedicated)" value={req.Network} onChange={(e) => set("Network", e.target.value)} />
        <input className="input w-24" placeholder="memory" value={req.Memory} onChange={(e) => set("Memory", e.target.value)} />
        <input className="input w-20" placeholder="cpus" value={req.CPUs} onChange={(e) => set("CPUs", e.target.value)} />
        <input className="input w-28" placeholder="max cost $" title="Cost budget in USD, recorded on the agent (advisory)"
          value={req.MaxCost} onChange={(e) => set("MaxCost", e.target.value)} />
        <input className="input w-32" placeholder="aws profile" title="Inject a ~/.aws profile as env credentials (tmpfs-backed; refresh with aws-refresh)"
          value={req.AWSProfile} onChange={(e) => set("AWSProfile", e.target.value)} />
      </div>
      {(!memOk || !cpuOk) && (
        <div className="text-xs text-yellow-500">
          {!memOk && <div>memory looks off — try like 8g or 512m</div>}
          {!cpuOk && <div>cpus should be a plain number like 4</div>}
        </div>
      )}
      <div className="flex gap-2">
        <button className="btn disabled:opacity-50" disabled={!!busy} onClick={() => submit(true)}>
          {busy === "dry" ? "Validating…" : "Dry-run preview"}
        </button>
        <button className="btn bg-green-800 hover:bg-green-700 disabled:opacity-50"
          disabled={!canSpawn}
          title={req.Worktree && !req.WorktreeDir ? "pick the local repo folder first" : ""}
          onClick={() => submit(false)}>
          {busy === "spawn" ? "Spawning…" : "Spawn"}
        </button>
      </div>
      {preview && <pre className="whitespace-pre-wrap rounded bg-neutral-900 p-3 text-xs">{preview}</pre>}
    </div>
  );
}
