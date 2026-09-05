@echo off
setlocal EnableExtensions
cd /d "%~dp0.."

if not exist backups mkdir backups

for /f %%i in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMdd-HHmmss"') do set STAMP=%%i
set RAW=backups\autoimport-%STAMP%.sql.gz
set ENC=backups\autoimport-%STAMP%.sql.gz.enc

echo Дамп autoimport из Ubuntu WSL :5433
wsl -d Ubuntu -u postgres -- bash -lc "pg_dump -p 5433 -d autoimport --no-owner --no-acl" | gzip > "%RAW%"
if errorlevel 1 (
  echo Не удалось снять дамп. Проверьте: wsl, postgres на 5433, база autoimport.
  exit /b 1
)

echo Записан %RAW%

if defined BACKUP_PASSPHRASE (
  echo Шифрование AES-256-CBC...
  openssl enc -aes-256-cbc -salt -pbkdf2 -in "%RAW%" -out "%ENC%" -pass env:BACKUP_PASSPHRASE
  if errorlevel 1 (
    echo openssl enc не удался. Дамп без шифрования остался в %RAW%.
    exit /b 1
  )
  del /f /q "%RAW%"
  echo Зашифрован %ENC%
  echo Расшифровка: openssl enc -d -aes-256-cbc -pbkdf2 -in %ENC% -out restore.sql.gz -pass env:BACKUP_PASSPHRASE
)

endlocal
