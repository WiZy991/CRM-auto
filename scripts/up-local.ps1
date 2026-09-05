<#
.SYNOPSIS
    Поднять локальную CRM на Windows + Ubuntu WSL (без Docker).

.DESCRIPTION
    Аналог `manage.py runserver`, только здесь четыре процесса:
    Postgres/Redis в WSL, туннель портов, API на :8080, Vite на :5173.

    Откроет отдельные окна PowerShell. Закройте их, когда закончите.

.EXAMPLE
    .\scripts\up-local.ps1
#>

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$backend = Join-Path $root 'backend'
$frontend = Join-Path $root 'frontend'

function Test-LocalPort([int]$Port) {
    $c = New-Object System.Net.Sockets.TcpClient
    try {
        $iar = $c.BeginConnect('127.0.0.1', $Port, $null, $null)
        $ok = $iar.AsyncWaitHandle.WaitOne(800, $false)
        return [bool]($ok -and $c.Connected)
    }
    catch {
        return $false
    }
    finally {
        $c.Close()
    }
}

Write-Host ''
Write-Host '  1/4  Postgres и Redis в Ubuntu WSL' -ForegroundColor Cyan
wsl -d Ubuntu -u root -- bash -lc 'service postgresql start >/dev/null; service redis-server start >/dev/null; ss -lntp | grep -E "5433|6380" || true'
if ($LASTEXITCODE -ne 0) {
    throw 'WSL Ubuntu недоступен. Проверьте: wsl -d Ubuntu'
}

Write-Host '  2/4  Туннель Windows → WSL (порты 15433 и 16380)' -ForegroundColor Cyan
if (-not (Test-LocalPort 15433) -or -not (Test-LocalPort 16380)) {
    Start-Process powershell -WorkingDirectory $backend -ArgumentList @(
        '-NoExit', '-Command',
        "Set-Location -LiteralPath '$backend'; Write-Host 'Туннель БД 127.0.0.1:15433 / 16380. Окно не закрывать.' -ForegroundColor Yellow; go run ./cmd/devproxy"
    )
    Start-Sleep -Seconds 3
}

Write-Host '  3/4  API :8080' -ForegroundColor Cyan
if (Test-LocalPort 8080) {
    Write-Host '       уже слушает, пропускаю' -ForegroundColor DarkGray
}
else {
    Start-Process powershell -WorkingDirectory $backend -ArgumentList @(
        '-NoExit', '-Command',
        "Set-Location -LiteralPath '$backend'; Write-Host 'API http://127.0.0.1:8080  Окно не закрывать.' -ForegroundColor Yellow; go run ./cmd/api"
    )
}

Write-Host '  4/4  Фронтенд :5173' -ForegroundColor Cyan
if (Test-LocalPort 5173) {
    Write-Host '       уже слушает, пропускаю' -ForegroundColor DarkGray
}
else {
    $npm = if (Get-Command npm.cmd -ErrorAction SilentlyContinue) { 'npm.cmd' } else { 'npm' }
    Start-Process powershell -WorkingDirectory $frontend -ArgumentList @(
        '-NoExit', '-Command',
        "Set-Location -LiteralPath '$frontend'; Write-Host 'Сайт http://localhost:5173  Окно не закрывать.' -ForegroundColor Yellow; & '$npm' run dev"
    )
}

Write-Host ''
Write-Host '  Откройте в браузере:  http://localhost:5173' -ForegroundColor Green
Write-Host '  API:                  http://127.0.0.1:8080/healthz'
Write-Host ''
Write-Host '  Если вход/регистрация дают 500 — не закрывайте окно туннеля'
Write-Host '  и в Ubuntu выполните:  sudo service postgresql start && sudo service redis-server start'
Write-Host ''
