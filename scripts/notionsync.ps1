# Sync Notion cookies from local Chrome to NotionChat server
param(
    [string]$Cdp = "http://127.0.0.1:9222",
    [string]$Url = "https://notion.rizky.app",
    [string]$Space = "38298ed3-0d46-8144-a1eb-00033da67864"
)
$repo = Split-Path -Parent $PSScriptRoot
Set-Location $repo
go run ./cmd/notionsync --cdp $Cdp --url $Url --space $Space