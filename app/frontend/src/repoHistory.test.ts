import { describe, expect, it } from "vitest";
import { recordRepoUse, topRepos, forgetRepo } from "./repoHistory";

describe("repoHistory", () => {
  it("sorts by use count, then recency", () => {
    recordRepoUse("git@github.com:o/a.git", 1000);
    recordRepoUse("git@github.com:o/b.git", 2000);
    recordRepoUse("git@github.com:o/b.git", 3000);
    recordRepoUse("git@github.com:o/c.git", 4000);
    expect(topRepos(3)).toEqual([
      "git@github.com:o/b.git", // 2 uses
      "git@github.com:o/c.git", // 1 use, newer
      "git@github.com:o/a.git", // 1 use, older
    ]);
  });

  it("ignores empty urls and supports forget", () => {
    recordRepoUse("   ");
    forgetRepo("git@github.com:o/a.git");
    expect(topRepos(10)).not.toContain("git@github.com:o/a.git");
  });
});
