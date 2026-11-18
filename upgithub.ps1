param(
    [string]$Message = "Update from CoreChain workspace"
)

$repos = @(
    @{ Name = "CoreChain";  Path = "." },
    @{ Name = "LearnChain"; Path = "..\DApps" },
    @{ Name = "WebChain";   Path = "..\WebChain" }
)

foreach ($r in $repos) {
    if (-not (Test-Path (Join-Path $r.Path ".git"))) {
        Write-Host "[$($r.Name)] Not a git repo, skipping." -ForegroundColor DarkGray
        continue
    }
    Write-Host "=== $($r.Name) ===" -ForegroundColor Cyan
    Push-Location $r.Path
    $status = git status --porcelain
    if ($status) {
        git add -A
        git commit -m $Message
    } else {
        Write-Host "No local changes to commit." -ForegroundColor Gray
    }
    git push origin main
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[$($r.Name)] Push failed (make sure the GitHub repo exists)." -ForegroundColor Red
    } else {
        Write-Host "[$($r.Name)] Pushed to GitHub successfully!" -ForegroundColor Green
    }
    Pop-Location
}
