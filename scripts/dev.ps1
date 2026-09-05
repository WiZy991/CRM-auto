<#
.SYNOPSIS
    Команды разработки для Windows.

.DESCRIPTION
    Замена Makefile для PowerShell: на Windows утилита make обычно не
    установлена, а ставить её ради нескольких команд не нужно.

.EXAMPLE
    .\scripts\dev.ps1 infra-up
    .\scripts\dev.ps1 migrate
    .\scripts\dev.ps1 test
#>

param(
    [Parameter(Position = 0)]
    [string]$Command = 'help',

    [Parameter(Position = 1, ValueFromRemainingArguments = $true)]
    [string[]]$Rest
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$backend = Join-Path $root 'backend'
$frontend = Join-Path $root 'frontend'

function Invoke-In($path, $exe, $arguments) {
    Push-Location $path
    try {
        & $exe @arguments
        if ($LASTEXITCODE -ne 0) {
            throw "$exe $($arguments -join ' ') завершилась с кодом $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

function Show-Help {
    Write-Host ''
    Write-Host '  Команды разработки платформы' -ForegroundColor Cyan
    Write-Host ''
    $commands = [ordered]@{
        'infra-up'       = 'Поднять postgres, redis, mailpit'
        'infra-down'     = 'Остановить инфраструктуру'
        'infra-logs'     = 'Смотреть логи инфраструктуры'
        'up'             = 'Поднять всё окружение целиком'
        'down'           = 'Остановить всё окружение'
        'reset'          = 'Полный сброс вместе с данными (необратимо)'
        'migrate'        = 'Применить миграции'
        'migrate-down'   = 'Откатить последнюю миграцию'
        'migrate-status' = 'Состояние миграций'
        'seed'           = 'Загрузить демонстрационные данные'
        'psql'           = 'Консоль psql в контейнере'
        'api'            = 'Запустить API локально'
        'worker'         = 'Запустить обработчик фоновых задач'
        'build'          = 'Собрать бинарники бэкенда'
        'test'           = 'Тесты бэкенда'
        'cover'          = 'Тесты с покрытием'
        'lint'           = 'go vet и gofmt'
        'audit'          = 'Поиск уязвимостей в зависимостях'
        'fe-install'     = 'Установить зависимости фронтенда'
        'fe-dev'         = 'Запустить фронтенд'
        'fe-build'       = 'Собрать фронтенд'
        'fe-test'        = 'Тесты фронтенда'
        'fe-lint'        = 'Проверки фронтенда'
        'secrets'        = 'Сгенерировать значения секретов для .env'
    }
    foreach ($key in $commands.Keys) {
        Write-Host ('    {0,-16} {1}' -f $key, $commands[$key])
    }
    Write-Host ''
}

# Генерация секретов выполняется локально и печатается в консоль:
# записывать их в файл автоматически нельзя, иначе легко перезаписать
# рабочую конфигурацию.
function New-Secrets {
    Write-Host ''
    Write-Host '  Сгенерированные значения. Скопируйте их в .env вручную:' -ForegroundColor Yellow
    Write-Host ''

    function New-RandomBase64([int]$bytes) {
        $buffer = New-Object 'System.Byte[]' $bytes
        [System.Security.Cryptography.RandomNumberGenerator]::Fill($buffer)
        return [Convert]::ToBase64String($buffer)
    }

    Write-Host ('    APP_JWT_SECRET={0}' -f (New-RandomBase64 48))
    Write-Host ('    APP_PII_ENCRYPTION_KEY={0}' -f (New-RandomBase64 32))
    Write-Host ('    APP_DB_PASSWORD={0}' -f (New-RandomBase64 24))
    Write-Host ('    APP_DB_MIGRATOR_PASSWORD={0}' -f (New-RandomBase64 24))
    Write-Host ('    APP_REDIS_PASSWORD={0}' -f (New-RandomBase64 24))
    Write-Host ''
}

switch ($Command) {
    'help' { Show-Help }

    'infra-up' { Invoke-In $root 'docker' @('compose', '--profile', 'infra', 'up', '-d') }
    'infra-down' { Invoke-In $root 'docker' @('compose', '--profile', 'infra', 'down') }
    'infra-logs' { Invoke-In $root 'docker' @('compose', '--profile', 'infra', 'logs', '-f', '--tail=100') }
    'up' { Invoke-In $root 'docker' @('compose', '--profile', 'full', 'up', '-d', '--build') }
    'down' { Invoke-In $root 'docker' @('compose', '--profile', 'full', 'down') }
    'reset' { Invoke-In $root 'docker' @('compose', '--profile', 'full', 'down', '-v') }
    'psql' { Invoke-In $root 'docker' @('compose', 'exec', 'postgres', 'psql', '-U', 'autoimport_app', '-d', 'autoimport') }

    'migrate' { Invoke-In $backend 'go' @('run', './cmd/migrate', 'up') }
    'migrate-down' { Invoke-In $backend 'go' @('run', './cmd/migrate', 'down', '1') }
    'migrate-status' { Invoke-In $backend 'go' @('run', './cmd/migrate', 'status') }
    'seed' { Invoke-In $backend 'go' @('run', './cmd/seed') }

    'api' { Invoke-In $backend 'go' @('run', './cmd/api') }
    'worker' { Invoke-In $backend 'go' @('run', './cmd/worker') }
    'test' { Invoke-In $backend 'go' @('test', './...', '-count=1') }
    'cover' { Invoke-In $backend 'go' @('test', './...', '-coverprofile=coverage.out', '-covermode=atomic') }
    'lint' {
        Invoke-In $backend 'go' @('vet', './...')
        Invoke-In $backend 'gofmt' @('-l', '.')
    }
    'audit' { Invoke-In $backend 'go' @('run', 'golang.org/x/vuln/cmd/govulncheck@latest', './...') }
    'build' {
        Invoke-In $backend 'go' @('build', '-trimpath', '-o', 'bin/api.exe', './cmd/api')
        Invoke-In $backend 'go' @('build', '-trimpath', '-o', 'bin/worker.exe', './cmd/worker')
        Invoke-In $backend 'go' @('build', '-trimpath', '-o', 'bin/migrate.exe', './cmd/migrate')
    }

    'fe-install' { Invoke-In $frontend 'npm' @('install') }
    'fe-dev' { Invoke-In $frontend 'npm' @('run', 'dev') }
    'fe-build' { Invoke-In $frontend 'npm' @('run', 'build') }
    'fe-test' { Invoke-In $frontend 'npm' @('run', 'test') }
    'fe-lint' {
        Invoke-In $frontend 'npm' @('run', 'lint')
        Invoke-In $frontend 'npm' @('run', 'typecheck')
    }

    'secrets' { New-Secrets }

    default {
        Write-Host "Неизвестная команда: $Command" -ForegroundColor Red
        Show-Help
        exit 1
    }
}
