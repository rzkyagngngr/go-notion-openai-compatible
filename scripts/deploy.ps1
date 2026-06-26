# Deploy cepat ke server (SSH tunnel mpds@127.0.0.1)
# Usage: .\scripts\deploy.ps1
param(
    [string]$Host = "mpds@127.0.0.1",
    [string]$Repo = "/home/Code/project/go-notion-openai-compatible"
)

$cmd = "cd $Repo && git pull --ff-only && docker compose up -d --build && docker compose ps notionchat && docker compose logs --tail=8 notionchat"
Write-Host "Deploying to $Host ..."
ssh $Host $cmd