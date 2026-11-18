---
name: upgithub
description: >-
  Multi-repository GitHub sync workflow for CoreChain, LearnChain (..\DApps), and WebChain (..\WebChain).
  Activate whenever the user says "upgithub" to commit and push all related repositories modified in the current round.
---

# `upgithub` — Multi-Repository GitHub Sync Workflow

When the user says **`upgithub`**, follow these steps:

1. **Identify Modified Repositories:**
   - Check git status in:
     - `CoreChain` (`.`) -> `https://github.com/Npasak/CoreChain.git` (`main`)
     - `LearnChain` (`..\DApps`) -> `https://github.com/Npasak/Learnchain.git` (`main`)
     - `WebChain` (`..\WebChain`) -> `https://github.com/Npasak/webchain.git` (`main`)
2. **Sync Shared Block Fixes (if applicable):**
   - If changes were made in `CoreChain/blocks/learnchain` (such as sync, cover, or handler bug fixes) that also apply to standalone `LearnChain.exe`, check whether `..\DApps` should receive the corresponding update before pushing.
   - Same for `CoreChain/blocks/webchain` and `..\WebChain`.
3. **Verify Before Push:**
   - Ensure `build.ps1` and relevant `go test` checks pass if Go code was modified.
   - Ensure `.gitignore` excludes binaries (`*.exe`, `*.Blz`), databases (`*.db`), and `Pic/` media.
4. **Commit & Push All Affected Repos:**
   - Run `powershell -ExecutionPolicy Bypass -File .\upgithub.ps1 -Message "<concise summary of changes>"` (or commit and push each modified repo individually with descriptive commit messages).
   - Report the exact commit hash and push result for each repository.
