$ErrorActionPreference = "Stop"

Write-Host "=== CoreChain Build & .Blz Packager ===" -ForegroundColor Cyan

New-Item -ItemType Directory -Force -Path "packages", "data", "Pic\webchain", "Pic\learnchain", "Pic\docchain", "_build_tmp" | Out-Null

Write-Host "[1/5] Tidying Go modules..." -ForegroundColor Yellow
go mod tidy
Push-Location blocks\webchain; go mod tidy; Pop-Location
Push-Location blocks\learnchain; go mod tidy; Pop-Location

Write-Host "[2/5] Building CoreChain.exe..." -ForegroundColor Yellow
go build -ldflags="-s -w" -o CoreChain.exe ./cmd/corechain

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

Write-Host "`nBuild Complete!" -ForegroundColor Green
Get-ChildItem packages\*.Blz | Select-Object Name, Length
