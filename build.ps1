$ErrorActionPreference = "Stop"

Write-Host "=== CoreChain Build & .Blz Packager ===" -ForegroundColor Cyan

New-Item -ItemType Directory -Force -Path "packages", "data", "Pic\webchain", "Pic\learnchain", "Pic\docchain", "_build_tmp" | Out-Null

Write-Host "[1/5] Tidying Go modules..." -ForegroundColor Yellow
go mod tidy
Push-Location blocks\webchain; go mod tidy; Pop-Location
Push-Location blocks\learnchain; go mod tidy; Pop-Location

Write-Host "[2/5] Building CoreChain binaries (win-arm, win-x64, native)..." -ForegroundColor Yellow
$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "arm64"
go build -ldflags="-H=windowsgui -s -w" -o CoreChain-win-arm.exe ./cmd/corechain
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$env:GOARCH = "amd64"
go build -ldflags="-H=windowsgui -s -w" -o CoreChain-win-x64.exe ./cmd/corechain
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
try {
    Copy-Item -Force "CoreChain-win-arm.exe" "CoreChain.exe" -ErrorAction Stop
} catch {
    Write-Host "Note: CoreChain.exe is currently running; updated CoreChain-win-arm.exe instead." -ForegroundColor DarkYellow
}

Write-Host "[3/5] Building & packing webchain.Blz..." -ForegroundColor Yellow
$webStage = "_build_tmp\webchain"
New-Item -ItemType Directory -Force -Path $webStage | Out-Null
Copy-Item -Force "blocks\webchain\manifest.json" "$webStage\manifest.json"
Push-Location blocks\webchain
go build -ldflags="-s -w" -o "..\..\$webStage\block.exe" .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Pop-Location
go run ./cmd/blzpack $webStage "packages\webchain.Blz"

Write-Host "[4/5] Building & packing learnchain.Blz..." -ForegroundColor Yellow
$learnStage = "_build_tmp\learnchain"
New-Item -ItemType Directory -Force -Path $learnStage | Out-Null
Copy-Item -Force "blocks\learnchain\manifest.json" "$learnStage\manifest.json"
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$learnLdFlags = "-s -w -X elearning-sbt/version.Version=v1.3.22 -X elearning-sbt/version.Commit=corechain -X elearning-sbt/version.BuildDate=$BuildDate"
Push-Location blocks\learnchain
go build -ldflags="$learnLdFlags" -o "..\..\$learnStage\block.exe" .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Pop-Location
go run ./cmd/blzpack $learnStage "packages\learnchain.Blz"

Write-Host "[5/5] Building & packing docchain.Blz..." -ForegroundColor Yellow
$docStage = "_build_tmp\docchain"
New-Item -ItemType Directory -Force -Path $docStage | Out-Null
Copy-Item -Force "blocks\docchain\manifest.json" "$docStage\manifest.json"
go build -ldflags="-s -w" -o "$docStage\block.exe" ./blocks/docchain
go run ./cmd/blzpack $docStage "packages\docchain.Blz"

Remove-Item -Recurse -Force "_build_tmp"

# Generate root manifest.json for Background Auto-Updater
$coreVer = (Select-String -Path "internal\config\config.go" -Pattern 'const Version = "([^"]+)"').Matches[0].Groups[1].Value
$webSha = (Get-FileHash "packages\webchain.Blz" -Algorithm SHA256).Hash.ToLower()
$learnSha = (Get-FileHash "packages\learnchain.Blz" -Algorithm SHA256).Hash.ToLower()
$docSha = (Get-FileHash "packages\docchain.Blz" -Algorithm SHA256).Hash.ToLower()
$armSha = (Get-FileHash "CoreChain-win-arm.exe" -Algorithm SHA256).Hash.ToLower()
$x64Sha = (Get-FileHash "CoreChain-win-x64.exe" -Algorithm SHA256).Hash.ToLower()

$manifestObj = @{
    core_version = $coreVer
    core_arm_url = "https://github.com/Npasak/CoreChain/raw/main/CoreChain-win-arm.exe"
    core_arm_sha256 = $armSha
    core_x64_url = "https://github.com/Npasak/CoreChain/raw/main/CoreChain-win-x64.exe"
    core_x64_sha256 = $x64Sha
    published_at = $BuildDate
    blocks = @(
        @{
            id = "webchain"
            version = "1.0.0"
            download_url = "https://github.com/Npasak/CoreChain/raw/main/packages/webchain.Blz"
            sha256 = $webSha
        },
        @{
            id = "learnchain"
            version = "1.3.22"
            download_url = "https://github.com/Npasak/CoreChain/raw/main/packages/learnchain.Blz"
            sha256 = $learnSha
        },
        @{
            id = "docchain"
            version = "1.0.0"
            download_url = "https://github.com/Npasak/CoreChain/raw/main/packages/docchain.Blz"
            sha256 = $docSha
        }
    )
}
$manifestObj | ConvertTo-Json -Depth 4 | Set-Content -Encoding UTF8 "manifest.json"

Write-Host "`nBuild Complete! (CoreChain v$coreVer)" -ForegroundColor Green
Get-ChildItem packages\*.Blz | Select-Object Name, Length
