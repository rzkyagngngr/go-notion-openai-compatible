# Inject fresh token_v2 without opening the web UI.
# Usage: .\scripts\inject_token.ps1 -Token "v03%3A..." [-Url "http://127.0.0.1:8787"] [-ApiKey "sk-notionchat"]
param(
    [Parameter(Mandatory = $true)][string]$Token,
    [string]$Url = "http://127.0.0.1:8787",
    [string]$ApiKey = "sk-notionchat"
)

$body = @{ token_v2 = $Token } | ConvertTo-Json -Compress
$headers = @{
    Authorization = "Bearer $ApiKey"
    "Content-Type" = "application/json"
}
Invoke-RestMethod -Uri "$Url/api/session/inject" -Method POST -Headers $headers -Body $body
Write-Host "Token injected. Test: curl $Url/v1/chat/completions ..."