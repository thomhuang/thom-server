# Replaces the production coffee log with the test database's coffee tables.
#
#   .\scripts\migrate-coffee.ps1            # prompts before writing
#   .\scripts\migrate-coffee.ps1 -Force     # no prompt
#
# Only CoffeeEntries, CoffeeRoasters, and CoffeeGrinders are involved: they are
# exported data-only from thom-db-test, the current production rows are written
# to a timestamped backup, and production is then overwritten to match test.
# Other tables are never touched. Exporting from a scratch directory keeps a
# CF_API_TOKEN in .env.local from shadowing the wrangler OAuth session, so run
# `npx wrangler login` once beforehand.
[CmdletBinding()]
param(
  [switch]$Force,
  [string]$BackupDir = (Join-Path $env:TEMP 'coffee-migration-backups')
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$wrangler = Join-Path $repoRoot 'node_modules\.bin\wrangler.cmd'
if (-not (Test-Path $wrangler)) {
  throw "wrangler not found at $wrangler. Run 'npm install' first."
}

$testConfig = Join-Path $repoRoot 'wrangler.test.jsonc'
$prodConfig = Join-Path $repoRoot 'wrangler.jsonc'
$tables = @('CoffeeEntries', 'CoffeeRoasters', 'CoffeeGrinders')
$tableArgs = @()
foreach ($table in $tables) { $tableArgs += @('--table', $table) }

$workDir = Join-Path $env:TEMP 'coffee-migration-work'
New-Item -ItemType Directory -Force -Path $workDir, $BackupDir | Out-Null

$testDump = Join-Path $workDir 'test-coffee.sql'
$applyFile = Join-Path $workDir 'apply.sql'
$prodDump = Join-Path $workDir 'prod-coffee-after.sql'
$backupFile = Join-Path $BackupDir ("prod-coffee-{0}.sql" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))

Push-Location $workDir
try {
  Write-Host 'Exporting coffee data from thom-db-test...'
  & $wrangler d1 export thom-db-test -c $testConfig --remote --no-schema --skip-confirmation @tableArgs --output $testDump
  if ($LASTEXITCODE -ne 0) { throw 'test coffee export failed' }

  Write-Host "Backing up production coffee data to $backupFile..."
  & $wrangler d1 export thom-db -c $prodConfig --remote --no-schema --skip-confirmation @tableArgs --output $backupFile
  if ($LASTEXITCODE -ne 0) { throw 'production coffee backup failed' }
} finally {
  Pop-Location
}

if (-not $Force) {
  $answer = Read-Host "Replace the production coffee log with test data? Backup: $backupFile. Type 'yes'"
  if ($answer -ne 'yes') { Write-Host 'Aborted.'; exit 1 }
}

# The dump's INSERTs carry explicit ids, so the tables are cleared first. The
# sqlite_sequence row is dropped as well so the re-inserted ids set the next
# autoincrement value from the data, matching test exactly. The file is written
# as UTF-8 without a BOM so the first statement is not garbled.
$prefix = @"
PRAGMA defer_foreign_keys=TRUE;
DELETE FROM CoffeeEntries;
DELETE FROM CoffeeRoasters;
DELETE FROM CoffeeGrinders;
DELETE FROM sqlite_sequence WHERE name = 'CoffeeEntries';
"@
[System.IO.File]::WriteAllText($applyFile, $prefix, (New-Object System.Text.UTF8Encoding($false)))
$dump = [System.IO.File]::ReadAllBytes($testDump)
$out = [System.IO.File]::Open($applyFile, [System.IO.FileMode]::Append)
try { $out.Write($dump, 0, $dump.Length) } finally { $out.Dispose() }

Push-Location $workDir
try {
  Write-Host 'Applying test coffee data to thom-db...'
  & $wrangler d1 execute thom-db -c $prodConfig --remote --yes --file $applyFile
  if ($LASTEXITCODE -ne 0) { throw 'applying test coffee data to production failed' }

  Write-Host 'Verifying production matches test...'
  & $wrangler d1 export thom-db -c $prodConfig --remote --no-schema --skip-confirmation @tableArgs --output $prodDump
  if ($LASTEXITCODE -ne 0) { throw 'verification export failed' }
} finally {
  Pop-Location
}

if ((Get-FileHash $testDump -Algorithm SHA256).Hash -ne (Get-FileHash $prodDump -Algorithm SHA256).Hash) {
  throw "verification failed: the production coffee log still differs from test. Backup: $backupFile"
}

Write-Host 'Done. Production coffee log now matches test.'
