param(
    [string]$Message = "Update CoreChain & .Blz packages"
)

Write-Host "=== CoreChain ===" -ForegroundColor Cyan
$status = git status --porcelain
if ($status) {
    git add -A
    git commit -m $Message
} else {
    Write-Host "No local changes to commit." -ForegroundColor Gray
}
git push origin main
if ($LASTEXITCODE -ne 0) {
    Write-Host "[CoreChain] Push failed." -ForegroundColor Red
} else {
    Write-Host "[CoreChain] Pushed to GitHub successfully!" -ForegroundColor Green
}
