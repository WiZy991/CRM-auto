# Шифрованный бэкап PostgreSQL. Запускать с хоста, когда контейнер поднят.
# Требует: docker, gzip. Пароль суперпользователя берётся из .env.

$ErrorActionPreference = 'Stop'
Set-Location (Split-Path -Parent $PSScriptRoot)

if (-not (Test-Path .env)) { throw 'Нет файла .env' }

Get-Content .env | ForEach-Object {
    if ($_ -match '^\s*#' -or $_ -notmatch '=') { return }
    $name, $value = $_.Split('=', 2)
    Set-Item -Path "Env:$name" -Value $value.Trim()
}

$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$outDir = Join-Path $PSScriptRoot '..\backups'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$outFile = Join-Path $outDir "autoimport-$stamp.sql.gz"

docker exec autoimport-postgres pg_dump -U postgres -d $env:POSTGRES_DB --no-owner --no-acl |
    gzip |
    Set-Content -AsByteStream $outFile

Write-Host "Бэкап записан: $outFile"
