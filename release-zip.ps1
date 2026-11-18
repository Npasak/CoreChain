$ErrorActionPreference = "Stop"

Write-Host "=== CoreChain Ready-to-Use ZIP Packager ===" -ForegroundColor Cyan

$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$learnLdFlags = "-H=windowsgui -s -w -X elearning-sbt/version.Version=v1.3.22 -X elearning-sbt/version.Commit=corechain -X elearning-sbt/version.BuildDate=$BuildDate"

function Build-ArchZip {
    param(
        [string]$Arch,     # arm64 or amd64
        [string]$ExeSrc,   # CoreChain-win-arm.exe or CoreChain-win-x64.exe
        [string]$OutZip    # CoreChain-win-arm64.zip or CoreChain-win-x64.zip
    )

    Write-Host "`nBuilding bundle for $Arch -> $OutZip ..." -ForegroundColor Yellow
    $stage = "_zip_stage_$Arch"
    if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
    New-Item -ItemType Directory -Force -Path "$stage\packages", "$stage\data", "$stage\Pic\webchain", "$stage\Pic\learnchain", "$stage\Pic\docchain" | Out-Null

    Copy-Item -Force $ExeSrc "$stage\CoreChain.exe"
    Copy-Item -Force ".env.example" "$stage\.env.example"

    @"
CoreChain Decentralized Block Launcher
======================================
1. แตกไฟล์ ZIP นี้ไปยังโฟลเดอร์ที่ต้องการ (เช่น C:\CoreChain)
2. ดับเบิลคลิกที่ CoreChain.exe เพื่อเริ่มทำงาน (ไม่มีจอดำ ทำงานใน System Tray)
3. เปิดเบราว์เซอร์ไปที่ http://localhost (หรือ http://localhost:8080)
4. บัญชี Admin เริ่มต้น: admin / admin1234
"@ | Set-Content -Encoding UTF8 "$stage\README.txt"

    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = $Arch

    # WebChain
    $wTmp = "_zip_tmp_web_$Arch"
    New-Item -ItemType Directory -Force -Path $wTmp | Out-Null
    Copy-Item -Force "blocks\webchain\manifest.json" "$wTmp\manifest.json"
    Push-Location blocks\webchain
    go build -ldflags="-H=windowsgui -s -w" -o "..\..\$wTmp\block.exe" .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Pop-Location

    # LearnChain
    $lTmp = "_zip_tmp_learn_$Arch"
    New-Item -ItemType Directory -Force -Path $lTmp | Out-Null
    Copy-Item -Force "blocks\learnchain\manifest.json" "$lTmp\manifest.json"
    Push-Location blocks\learnchain
    go build -ldflags="$learnLdFlags" -o "..\..\$lTmp\block.exe" .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Pop-Location

    # DocChain
    $dTmp = "_zip_tmp_doc_$Arch"
    New-Item -ItemType Directory -Force -Path $dTmp | Out-Null
    Copy-Item -Force "blocks\docchain\manifest.json" "$dTmp\manifest.json"
    go build -ldflags="-H=windowsgui -s -w" -o "$dTmp\block.exe" ./blocks/docchain
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue

    # Pack .Blz using native blzpack
    go run ./cmd/blzpack $wTmp "$stage\packages\webchain.Blz"
    go run ./cmd/blzpack $lTmp "$stage\packages\learnchain.Blz"
    go run ./cmd/blzpack $dTmp "$stage\packages\docchain.Blz"

    if (Test-Path $OutZip) { Remove-Item -Force $OutZip }
    Compress-Archive -Path "$stage\*" -DestinationPath $OutZip -CompressionLevel Optimal

    Remove-Item -Recurse -Force $stage, $wTmp, $lTmp, $dTmp
    Write-Host "Created $OutZip successfully!" -ForegroundColor Green
}

Build-ArchZip -Arch "arm64" -ExeSrc "CoreChain-win-arm.exe" -OutZip "CoreChain-win-arm64.zip"
Build-ArchZip -Arch "amd64" -ExeSrc "CoreChain-win-x64.exe" -OutZip "CoreChain-win-x64.zip"

Get-ChildItem CoreChain-win-*.zip | Select-Object Name, Length
