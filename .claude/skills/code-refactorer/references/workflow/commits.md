# Commits

- Inspect status first; preserve all unrelated staged, unstaged, and untracked work.
- Make one coherent, behavior-preserving move per commit. Keep inseparable contract and caller changes together.
- Commit a needed characterization test separately, before the refactor.
- Keep bug fixes separate and only when authorized.
- Run the narrowest relevant check before each commit; every commit must be usable and buildable.
- Stage exact task-owned paths or hunks. Never use broad staging in a dirty worktree.
- Review the staged diff before committing and recheck status afterward.
- Use a concise message naming the structural outcome.
- Do not bypass hooks, rewrite history, or push unless separately authorized.
- If hooks, signing, identity, or permissions block the commit, report it; do not weaken checks or reconfigure Git.
- Report each commit hash, subject, verification, and any changes left uncommitted.
- Don't add your data in commit or write you as co-author and you must respact the user data and check the local git first if you get his information use it or you don't found data you must ask user first 