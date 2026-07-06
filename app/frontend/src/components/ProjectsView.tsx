import { useEffect, useState } from "react";
import { useStore } from "../store";
import { errText } from "../types";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";
import { Service } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/state";

type Project = { url: string; count: number; last: number };

const isLocal = (u: string) => /^[/~]/.test(u) || (!u.includes("://") && !u.includes("@"));
const shortName = (u: string) =>
  isLocal(u)
    ? u.replace(/^.*\//, "") || u
    : u.replace(/^(git@github\.com:|https:\/\/github\.com\/)/, "").replace(/\.git$/, "");

export function ProjectsView() {
  const { toast, run, setView, addPendingSpawn, removePendingSpawn } = useStore();
  const [projects, setProjects] = useState<Project[]>([]);
  const [newUrl, setNewUrl] = useState("");

  const reload = () => Service.Projects().then((p: Project[] | null) => setProjects(p ?? [])).catch(() => {});
  useEffect(() => { reload(); }, []);

  const addUrl = async () => {
    if (!newUrl.trim()) return;
    try { await Service.ProjectAdd(newUrl.trim()); setNewUrl(""); reload(); }
    catch (e) { toast(errText("add project", e)); }
  };

  const addFolder = async () => {
    try {
      const path = await AgentService.PickFolder();
      if (!path) return;
      await Service.ProjectAdd(path);
      reload();
      toast(`added ${shortName(path)}`);
    } catch (e) { toast(errText("pick folder", e)); }
  };

  const launch = async (p: Project, agent: string) => {
    const label = `${agent} · ${shortName(p.url)}`;
    const pid = addPendingSpawn(label);
    setView("agents"); // jump to the list; the "starting…" row shows immediately
    try {
      if (isLocal(p.url)) {
        await run(`Copying ${shortName(p.url)} into ${agent}`, AgentService.SpawnFromLocal(agent, p.url));
      } else {
        await Service.ProjectUse(p.url);
        await run(`Spawning ${label}`, AgentService.Spawn({ Agent: agent, Repo: p.url, SSH: true } as any));
      }
      setTimeout(() => AgentService.Refresh(), 1500);
    } catch { removePendingSpawn(pid); }
    finally { reload(); }
  };

  const remove = async (url: string) => {
    try { await Service.ProjectRemove(url); reload(); } catch (e) { toast(errText("remove", e)); }
  };

  return (
    <div className="flex max-w-3xl flex-col gap-4 p-6">
      <h2 className="text-lg">Projects</h2>

      <div className="flex gap-2">
        <input className="input flex-1" placeholder="repo URL (git@… or https://…)"
          value={newUrl} onChange={(e) => setNewUrl(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && addUrl()} />
        <button className="btn" onClick={addUrl}>Add repo</button>
        <button className="btn" onClick={addFolder}>📁 Add local folder…</button>
      </div>

      {projects.length === 0 && (
        <div className="text-neutral-500">No saved projects. Add a repo URL or a local folder above.</div>
      )}

      <div className="flex flex-col divide-y divide-neutral-800 rounded border border-neutral-800">
        {projects.map((p) => (
          <div key={p.url} className="flex items-center gap-3 px-3 py-2">
            <span className={`rounded px-1.5 py-0.5 text-xs ${isLocal(p.url) ? "bg-purple-900 text-purple-200" : "bg-neutral-800 text-neutral-400"}`}>
              {isLocal(p.url) ? "local" : "repo"}
            </span>
            <span className="min-w-0 flex-1 truncate text-sm" title={p.url}>{shortName(p.url)}</span>
            {p.count > 0 && <span className="text-xs text-neutral-600">×{p.count}</span>}
            <button className="btn" onClick={() => launch(p, "claude")}>▶ claude</button>
            <button className="btn" onClick={() => launch(p, "codex")}>codex</button>
            <button className="text-neutral-500 hover:text-red-400" title="forget" onClick={() => remove(p.url)}>✕</button>
          </div>
        ))}
      </div>

      <div className="text-xs text-neutral-600">
        Local folders are copied into a fresh agent's <code>/workspace</code> (heavy dirs like
        <code> node_modules</code>, <code>.git</code> are skipped). Repos are cloned via git.
      </div>
    </div>
  );
}
