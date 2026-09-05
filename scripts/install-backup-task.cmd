@echo off
setlocal EnableExtensions
cd /d "%~dp0.."
set SCRIPT=%~dp0backup-wsl.cmd
echo Ежедневный бэкап в 03:15 через Планировщик заданий Windows.
schtasks /Create /TN "CRM-auto-backup" /TR "\"%SCRIPT%\"" /SC DAILY /ST 03:15 /RU "%USERNAME%" /F
if errorlevel 1 (
  echo Не удалось создать задание. Запустите cmd от имени пользователя с правом schtasks.
  exit /b 1
)
echo Задание CRM-auto-backup создано. Ручной запуск: scripts\backup-wsl.cmd
endlocal
