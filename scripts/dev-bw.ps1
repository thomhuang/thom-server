# Runs the API locally with secrets resolved from Bitwarden.
#
#   .\scripts\dev-bw.ps1
#   .\scripts\dev-bw.ps1 -Addr :8080
#
# Each value in .env.bw names a Bitwarden login item; the item's password is
# injected into the server process only, so nothing is written to disk and the
# caller's shell is restored afterward. Non-secret config still comes from
# .env.local.
[CmdletBinding()]
param(
  [string]$Addr = ':4000',
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs
)

$ErrorActionPreference = 'Stop'

if (-not (Get-Command bw -ErrorAction SilentlyContinue)) {
  throw 'bw not found. Install it with: winget install -e --id Bitwarden.CLI'
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$mapPath = Join-Path $repoRoot '.env.bw'
if (-not (Test-Path $mapPath)) {
  throw "Missing $mapPath. See README.md for the Bitwarden setup."
}

# Ensure an unlocked Bitwarden session; unlock interactively if needed.
$status = (& bw status | ConvertFrom-Json).status
if ($status -eq 'unauthenticated') {
  throw 'Not logged in to Bitwarden. Run: bw login'
}
$ownSession = $false
if ($status -ne 'unlocked') {
  $env:BW_SESSION = & bw unlock --raw
  if (-not $env:BW_SESSION) { throw 'Bitwarden unlock failed.' }
  $ownSession = $true
}
& bw sync | Out-Null

# Read "ENV_VAR=Item Name" pairs, skipping blanks and comments.
$pairs = @()
foreach ($line in Get-Content $mapPath) {
  $text = $line.Trim()
  if ($text -and -not $text.StartsWith('#')) {
    $i = $text.IndexOf('=')
    if ($i -lt 1) { throw "Bad .env.bw line: $text" }
    $pairs += [pscustomobject]@{
      Name = $text.Substring(0, $i).Trim()
      Item = $text.Substring($i + 1).Trim()
    }
  }
}

# Resolve by exact item id or exact name, so a dev item and a similarly named
# PRD item cannot both match.
$vault = & bw list items | ConvertFrom-Json

$saved = @{}
try {
  foreach ($pair in $pairs) {
    $saved[$pair.Name] = [Environment]::GetEnvironmentVariable($pair.Name)
    $matches = @($vault | Where-Object {
      $_.id -eq $pair.Item -or $_.name -eq $pair.Item -or $_.login.username -eq $pair.Item
    })
    if ($matches.Count -ne 1) {
      throw "Bitwarden item '$($pair.Item)' matched $($matches.Count) items. Put its exact name, username, or id in .env.bw."
    }
    $value = $matches[0].login.password
    if ([string]::IsNullOrEmpty($value)) {
      throw "Bitwarden item '$($pair.Item)' has no password value."
    }
    Set-Item -Path "Env:$($pair.Name)" -Value $value
  }

  Push-Location $repoRoot
  try {
    & go run ./cmd/server "-addr=$Addr" @ExtraArgs
  } finally {
    Pop-Location
  }
} finally {
  foreach ($name in $saved.Keys) {
    if ($null -eq $saved[$name]) {
      Remove-Item -Path "Env:$name" -ErrorAction SilentlyContinue
    } else {
      Set-Item -Path "Env:$name" -Value $saved[$name]
    }
  }
  if ($ownSession) {
    Remove-Item Env:BW_SESSION -ErrorAction SilentlyContinue
  }
}
