import { useEffect, useState } from "react";
import { Play } from "lucide-react";
import { useStore } from "../store";
import { errText } from "../types";
import { topRepos } from "../repoHistory";
import { AgentService } from "../../bindings/github.com/0x666c6f/safe-agentic/app/internal/svc";

const SHORTCUTS: [string, string][] = [
  ["⌘K", "command palette"],
  ["⌘1…9", "jump to agent"],
  ["j / k", "move selection"],
  ["⌘T ⌘D ⌘O ⌘I", "terminal · diff · output · info"],
  ["⌘F", "search in terminal"],
  ["right-click row", "quick actions"],
];

export function EmptyState() {
  const { setView, toast } = useStore();
  const [repos, setRepos] = useState<string[]>([]);
  useEffect(() => { topRepos(6).then(setRepos).catch(() => {}); }, []);

  return (
    <div className="flex h-full flex-col items-center justify-center gap-6 text-neutral-400">
      <div className="text-lg text-neutral-300">No chat selected</div>
      <div className="flex gap-2">
        <button className="btn bg-green-800 hover:bg-green-700" onClick={() => setView("spawn")}>+ New chat</button>
        <button className="btn" onClick={() => setView("projects")}>Projects & local folders</button>
        <button className="btn" onClick={() => setView("fleet")}>Run a pipeline</button>
      </div>
      {repos.length > 0 && (
        <div className="flex max-w-lg flex-wrap justify-center gap-1">
          {repos.map((r) => (
            <button key={r} className="rounded bg-neutral-800 px-2 py-0.5 text-xs hover:bg-neutral-700"
              title={`Start a Claude chat on ${r}`}
              onClick={() => AgentService.Spawn({ Agent: "claude", Repo: r, SSH: true } as any)
                .then(() => toast(`spawning on ${r}`))
                .catch((e: unknown) => toast(errText("spawn", e)))}>
              <Play className="mr-1 inline h-2.5 w-2.5" />{r.replace(/^(git@github\.com:|https:\/\/github\.com\/)/, "").replace(/\.git$/, "")}
            </button>
          ))}
        </div>
      )}
      <table className="text-xs text-neutral-500">
        <tbody>
          {SHORTCUTS.map(([k, d]) => (
            <tr key={k}><td className="pr-4 text-right font-mono text-neutral-400">{k}</td><td>{d}</td></tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
