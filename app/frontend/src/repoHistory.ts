// Saved projects (repos) for the spawn form — backed by the Go-side store
// (~/.safe-ag/app-projects.json) shared with the systray Projects menu.
import { Service } from "../bindings/github.com/0x666c6f/safe-agentic/app/internal/state";

export async function recordRepoUse(url: string): Promise<void> {
  if (url.trim()) await Service.ProjectUse(url.trim());
}

export async function topRepos(n = 6): Promise<string[]> {
  const list = (await Service.Projects()) ?? [];
  return list.slice(0, n).map((p: any) => p.url);
}

export async function forgetRepo(url: string): Promise<void> {
  await Service.ProjectRemove(url);
}
