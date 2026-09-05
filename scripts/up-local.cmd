@echo off
setlocal
cd /d "%~dp0.."

echo [1/4] Postgres и Redis в Ubuntu WSL
wsl -d Ubuntu -u root -- bash -lc "service postgresql start >/dev/null; service redis-server start >/dev/null"

echo [2/4] Туннель 127.0.0.1:15433 / 16380
start "CRM tunnel" cmd /k "cd /d ""%~dp0..\backend"" && echo Туннель БД. Окно не закрывать. && go run ./cmd/devproxy"

timeout /t 4 /nobreak >nul

echo [3/4] API :8080
start "CRM API" cmd /k "cd /d ""%~dp0..\backend"" && echo API http://127.0.0.1:8080  Окно не закрывать. && go run ./cmd/api"

timeout /t 2 /nobreak >nul

echo [4/4] Сайт :5173
start "CRM UI" cmd /k "cd /d ""%~dp0..\frontend"" && echo Сайт http://localhost:5173  Окно не закрывать. && npm.cmd run dev"

echo.
echo Откройте http://localhost:5173
echo Если скрипт .ps1 не запускается из-за ExecutionPolicy — пользуйтесь этим .cmd
echo.
endlocal
