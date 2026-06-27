# Deploy cepat via PuTTY plink (non-interaktif)
# Usage: .\scripts\deploy.ps1
param(
    [string]$Plink = "C:\Program Files\PuTTY\plink.exe",
    [string]$User = "mpds@localhost",
    [string]$Password = "mpds",
    [string]$Repo = "/home/Code/project/go-notion-openai-compatible"
)

$remote = "cd $Repo && git pull --ff-only origin main && docker compose up -d --build && docker compose ps notionchat && curl -s http://127.0.0.1:8787/healthz && echo && curl -s http://127.0.0.1:8787/api/session | head -c 400"
Write-Host "git push (local) ..."
git push origin main
Write-Host "Deploy via plink -> $User ..."
echo y | & $Plink -ssh $User -pw $Password -batch $remote