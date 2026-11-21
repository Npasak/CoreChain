---
name: upgithub
description: >-
  CoreChain build & GitHub sync workflow.
  Activate whenever the user says "upgithub" to bump versions, build CoreChain Windows binaries (ARM64 + x64) and .Blz block packages (webchain, learnchain, docchain, ai), enforce privacy/secret-hygiene rules across all current and future blocks, and commit/push CoreChain to GitHub.
---

# `upgithub` — CoreChain Build & GitHub Sync Workflow

When the user says **`upgithub`**, follow these steps:

1. **Bump Versions (if code changed in this round):**
   - `CoreChain`: bump `Version` in `internal/config/config.go` (and `manifest.json` via `build.ps1`).
2. **Privacy & Build-Time Secret Injection Rule (Applies to ALL Current & Future Blocks):**
   - **Never hardcode personal IDs, URLs, or secrets in tracked source files** (`*.go`, `*.gs`, `*.js`, `*.html`, `*.json`, etc.) — including Google Drive Folder IDs (`PICSYNC_ROOT_FOLDER_ID`, `COVERS_DRIVE_FOLDER_ID`), personal Google Apps Script URLs (`https://script.google.com/macros/s/AKfycb.../exec`), API keys, internal LAN IPs, or personal emails/accounts.
   - **Inject needed defaults into `.Blz` at build time:** Store personal/deployment defaults (such as `LEARNCHAIN_GAS_URL` or future block sync/endpoint variables) in `.env` (gitignored), declare them as `var ... = ""` in Go, and inject them in `build.ps1` via `-ldflags "-X ..."` when building each `.Blz` package.
   - **Centralized Member System:** Blocks must not include standalone member registration/management systems or seed personal/demo user accounts when running under CoreChain.
3. **Build CoreChain Binaries & `.Blz` Block Packages:**
   - Run `powershell -ExecutionPolicy Bypass -File .\build.ps1` to produce `CoreChain-win-arm.exe`, `CoreChain-win-x64.exe`, `CoreChain.exe`, and `packages/*.Blz` (`webchain.Blz`, `learnchain.Blz`, `docchain.Blz`, `ai.Blz`, plus any future blocks).
   - Do **NOT** build, commit, or upload standalone `.exe` binaries for `webchain`, `learnchain`, `docchain`, or future blocks — blocks are distributed exclusively as `.Blz` packages.
4. **Pre-Push Verify & Clean:**
   - Ensure `.gitignore` tracks `CoreChain-win-arm.exe`, `CoreChain-win-x64.exe`, and `packages/*.Blz`, while excluding `CoreChain.exe`, `*.zip`, `.env`, databases (`*.db*`), `*.sync.json`, `backups/`, `data/`, `installed_blocks/`, and `Pic/` media.
   - Audit staged/tracked text files before pushing to ensure zero hardcoded personal Drive IDs, GAS `AKfycb...` URLs, or `.sync.json` files are included.
5. **Commit & Push CoreChain:**
   - Commit & push `CoreChain` (`https://github.com/Npasak/CoreChain.git`) on `main`.
   - Report the build outputs, version numbers, and git push results.
