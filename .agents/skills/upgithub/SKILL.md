---
name: upgithub
description: >-
  CoreChain build & GitHub sync workflow.
  Activate whenever the user says "upgithub" to bump versions, build CoreChain Windows binaries (ARM64 + x64) and .Blz block packages (webchain, learnchain, docchain, ai), and commit/push CoreChain to GitHub.
---

# `upgithub` — CoreChain Build & GitHub Sync Workflow

When the user says **`upgithub`**, follow these steps:

1. **Bump Versions (if code changed in this round):**
   - `CoreChain`: bump `Version` in `internal/config/config.go` (and `manifest.json` if needed)
2. **Build CoreChain Binaries & `.Blz` Block Packages:**
   - Run `powershell -ExecutionPolicy Bypass -File .\build.ps1` to produce `CoreChain-win-arm.exe`, `CoreChain-win-x64.exe`, `CoreChain.exe`, and `packages/*.Blz` (`webchain.Blz`, `learnchain.Blz`, `docchain.Blz`, `ai.Blz`).
   - Do **NOT** build or upload standalone `.exe` binaries for `webchain`, `learnchain`, or `docchain` — blocks are distributed exclusively as `.Blz` packages.
3. **Verify & Clean:**
   - Ensure `.gitignore` tracks `CoreChain-win-arm.exe`, `CoreChain-win-x64.exe`, and `packages/*.Blz`, while excluding `CoreChain.exe`, `*.zip`, databases (`*.db`), `backups/`, `data/`, and `Pic/` media.
4. **Commit & Push CoreChain:**
   - Commit & push `CoreChain` (`https://github.com/Npasak/CoreChain.git`) on `main`.
   - Report the build outputs, version numbers, and git push results.
