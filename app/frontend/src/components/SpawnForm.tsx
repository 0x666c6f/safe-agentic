import { useEffect, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { useStore } from "../store";
import { errText } from "../types";
import { recordRepoUse, topRepos, forgetRepo } from "../repoHistory";

export function SpawnForm() {
  const { toast, setView } = useStore();
  const [req, setReq] = useState({
    Agent: "claude", Name: "", Repo: "", Prompt: "", Template: "", Network: "",
    Memory: "", CPUs: "", SSH: true, ReuseAuth: false, Worktree: false, DryRun: false,
  });
  const [templates, setTemplates] = useState<string[]>([]);
  const [preview, setPreview] = useState("");
  const [savedRepos, setSavedRepos] = useState<string[]>(() => topRepos(6));

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
    try {
      const out = await AgentService.Spawn({ ...req, DryRun: dryRun } as any);
      if (dryRun) setPreview(out);
      else {
        if (req.Repo) { recordRepoUse(req.Repo); setSavedRepos(topRepos(6)); }
        toast(`spawned:\n${out.trim().split("\n").slice(-3).join("\n")}`);
        setView("agents");
      }
    } catch (e) { toast(errText(dryRun ? "dry-run" : "spawn", e)); }
  };

  const Check = ({ k, label }: { k: "SSH" | "ReuseAuth" | "Worktree"; label: string }) => (
    <label className="flex items-center gap-2 text-sm">
      <input type="checkbox" checked={req[k]} onChange={(e) => set(k, e.target.checked)} /> {label}
    </label>
  );

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
      <input className="input" placeholder="repo URL (optional — empty gives a blank workspace)" value={req.Repo}
        onChange={(e) => set("Repo", e.target.value)} />
      {savedRepos.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {savedRepos.map((r) => (
            <span key={r} className="flex items-center gap-1 rounded bg-neutral-800 px-2 py-0.5 text-xs">
              <button className={req.Repo === r ? "text-blue-300" : "hover:text-blue-300"}
                title={r} onClick={() => set("Repo", r)}>
                {r.replace(/^(git@github\.com:|https:\/\/github\.com\/)/, "").replace(/\.git$/, "")}
              </button>
              <button className="text-neutral-500 hover:text-red-400" title="forget"
                onClick={() => { forgetRepo(r); setSavedRepos(topRepos(6)); }}>✕</button>
            </span>
          ))}
        </div>
      )}
      <textarea className="input min-h-24" placeholder="prompt (optional)" value={req.Prompt}
        onChange={(e) => set("Prompt", e.target.value)} />
      <select className="input" value={req.Template} onChange={(e) => set("Template", e.target.value)}>
        <option value="">no template</option>
        {templates.map((t) => <option key={t} value={t}>{t}</option>)}
      </select>
      <div className="flex gap-4"><Check k="SSH" label="--ssh" /><Check k="ReuseAuth" label="--reuse-auth" /><Check k="Worktree" label="--worktree" /></div>
      <div className="flex gap-2">
        <input className="input flex-1" placeholder="network (blank = dedicated)" value={req.Network} onChange={(e) => set("Network", e.target.value)} />
        <input className="input w-24" placeholder="memory" value={req.Memory} onChange={(e) => set("Memory", e.target.value)} />
        <input className="input w-20" placeholder="cpus" value={req.CPUs} onChange={(e) => set("CPUs", e.target.value)} />
      </div>
      <div className="flex gap-2">
        <button className="btn" onClick={() => submit(true)}>Dry-run preview</button>
        <button className="btn bg-green-800 hover:bg-green-700" onClick={() => submit(false)}>Spawn</button>
      </div>
      {preview && <pre className="whitespace-pre-wrap rounded bg-neutral-900 p-3 text-xs">{preview}</pre>}
    </div>
  );
}
