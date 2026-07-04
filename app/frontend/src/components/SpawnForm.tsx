import { useEffect, useState } from "react";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { useStore } from "../store";

export function SpawnForm() {
  const { toast, setView } = useStore();
  const [req, setReq] = useState({
    Agent: "claude", Repo: "", Prompt: "", Template: "", Network: "",
    Memory: "", CPUs: "", SSH: true, ReuseAuth: false, Worktree: false, DryRun: false,
  });
  const [templates, setTemplates] = useState<string[]>([]);
  const [preview, setPreview] = useState("");

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
      else { toast(`spawned:\n${out}`); setView("agents"); }
    } catch (e) { toast(String(e)); }
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
            className={`btn ${req.Agent === t ? "bg-neutral-600" : ""}`}>{t}</button>
        ))}
      </div>
      <input className="input" placeholder="repo URL (git@… or https://…)" value={req.Repo}
        onChange={(e) => set("Repo", e.target.value)} />
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
        <button className="btn" disabled={!req.Repo} onClick={() => submit(true)}>Dry-run preview</button>
        <button className="btn bg-green-800 hover:bg-green-700" disabled={!req.Repo} onClick={() => submit(false)}>Spawn</button>
      </div>
      {preview && <pre className="whitespace-pre-wrap rounded bg-neutral-900 p-3 text-xs">{preview}</pre>}
    </div>
  );
}
