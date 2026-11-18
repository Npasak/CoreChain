---
name: upgithub
description: >-
  Multi-repository GitHub sync workflow for CoreChain, LearnChain (..\DApps), and WebChain (..\WebChain).
  Activate whenever the user says "upgithub" to bump versions, build 2-architecture Windows binaries (ARM64 + x64) for all 3 apps, and commit/push all related repositories.
---

# `upgithub` — Multi-Repository Build & GitHub Sync Workflow

When the user says **`upgithub`**, follow these steps:

1. **Bump Versions (if code changed in this round):**
   - `CoreChain`: bump `Version` in `internal/config/config.go`
   - `LearnChain` (`..\DApps`): bump version tag / `version/version.go`
   - `WebChain` (`..\WebChain`): bump version in `version/version.go`
2. **Build 2 Architectures (`arm64` + `x64`) for Each Program:**
   - **CoreChain:** Run `powershell -ExecutionPolicy Bypass -File .\build.ps1` to produce `CoreChain-win-arm.exe`, `CoreChain-win-x64.exe`, `CoreChain.exe`, and `packages/*.Blz`.
   - **LearnChain (`..\DApps`):** Build both `LearnChain-arm64.exe` and `LearnChain-x64.exe` (via `.\build.ps1` or `GOOS=windows GOARCH=arm64|amd64`).
   - **WebChain (`..\WebChain`):** Build both `WebChain-windows-arm64.exe` and `WebChain-windows-amd64.exe` (via `.\build.ps1` or `GOOS=windows GOARCH=arm64|amd64`).
3. **Verify & Clean:**
   - Ensure `.gitignore` excludes binaries (`*.exe`, `*.Blz`), databases (`*.db`), and `Pic/` media.
4. **Commit & Push All Affected Repos:**
   - Commit & push `CoreChain` (`https://github.com/Npasak/CoreChain.git`), `LearnChain` (`https://github.com/Npasak/Learnchain.git`), and `WebChain` (`https://github.com/Npasak/webchain.git`) on `main`.
   - Report the build outputs, version numbers, and git push results.
