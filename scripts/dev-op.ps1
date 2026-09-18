# Runs the API locally with secrets injected from 1Password.
#
#   .\scripts\dev-op.ps1
#   .\scripts\dev-op.ps1 -Addr :8080
#
# `op run` resolves the op:// references in .env.op and injects the values into
# the server process only, so nothing is written to disk and the caller's shell
# is left untouched. Non-secret config still comes from .env.local.
[CmdletBinding()]
param(
  [string]$Addr = ':4000',
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$ExtraArgs
)

$ErrorActionPreference = 'Stop'

if (-not (Get-Command op -ErrorAction SilentlyContinue)) {
  throw 'op not found. Install it with: winget install --id AgileBits.1Password.CLI'
}

$repoRoot = Split-Path -Parent $PSScriptRoot

Push-Location $repoRoot
try {
  & op run --env-file="$repoRoot\.env.op" -- go run ./cmd/server "-addr=$Addr" @ExtraArgs
} finally {
  Pop-Location
}
