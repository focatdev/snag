@echo off
setlocal
cd /d "%~dp0"

rem usage: build.bat          release build, bin\Snag.exe
rem        build.bat debug    console window with logs and devtools, bin\Snag-debug.exe

set MODE=%~1
if "%MODE%"=="" set MODE=release

rem the release workflow sets these, local builds stay 0.1.0 and never update themselves
if "%SNAG_VERSION%"=="" set SNAG_VERSION=0.1.0
set LDX=-X main.appVersion=%SNAG_VERSION% -X main.updateRepo=%SNAG_REPO%

where go >nul 2>nul
if errorlevel 1 (
  echo Go is not installed. Get it from https://go.dev/dl/
  exit /b 1
)
where npm >nul 2>nul
if errorlevel 1 (
  echo Node.js is not installed. Get it from https://nodejs.org/
  exit /b 1
)

echo.
echo [1/5] Installing frontend packages
pushd frontend
rem install every time so new dependencies land, it is quick when nothing changed
call npm install --no-audit --no-fund
if errorlevel 1 goto :npmfail

echo.
echo [2/5] Building frontend
call npm run build
if errorlevel 1 goto :npmfail
popd

echo.
echo [3/5] Fetching Go modules
set GOTOOLCHAIN=auto
go mod tidy
if errorlevel 1 (
  echo go mod tidy failed. Check your internet connection.
  exit /b 1
)

echo.
echo [4/5] Building Snag
rem regenerated every build so icon changes always land
if exist rsrc_windows_amd64.syso del rsrc_windows_amd64.syso
(
  go run github.com/tc-hib/go-winres@v0.3.3 simply --icon build\windows\icon.ico --manifest gui --arch amd64 --product-version %SNAG_VERSION% --file-version %SNAG_VERSION% --file-description Snag --product-name Snag --original-filename Snag.exe
  if errorlevel 1 echo Could not embed the app icon, building without it.
)
if not exist bin mkdir bin
if /i "%MODE%"=="debug" (
  go build -ldflags "%LDX%" -o bin\Snag-debug.exe .
) else (
  go build -tags production -trimpath -buildvcs=false -ldflags "-w -s -H windowsgui %LDX%" -o bin\Snag.exe .
)
if errorlevel 1 (
  echo Go build failed. Copy the errors above and send them over.
  exit /b 1
)

if /i "%MODE%"=="debug" (
  echo.
  echo Done. Run bin\Snag-debug.exe to see logs in this console.
  exit /b 0
)

echo.
echo [5/5] Building the installer
if exist installer\ui rmdir /S /Q installer\ui
xcopy /E /I /Q /Y frontend\dist installer\ui >nul
if not exist installer\payload mkdir installer\payload
copy /Y bin\Snag.exe installer\payload\Snag.exe >nul
pushd installer
if exist rsrc_windows_amd64.syso del rsrc_windows_amd64.syso
go run github.com/tc-hib/go-winres@v0.3.3 simply --icon ..\build\windows\icon.ico --manifest gui --arch amd64 --product-version %SNAG_VERSION% --file-version %SNAG_VERSION% --file-description "Snag Setup" --product-name Snag --original-filename SnagSetup.exe
popd
go build -tags production -trimpath -buildvcs=false -ldflags "-w -s -H windowsgui -X main.version=%SNAG_VERSION%" -o bin\SnagSetup.exe ./installer
if errorlevel 1 (
  echo Installer build failed. Snag.exe itself is fine in bin.
  exit /b 1
)

echo.
echo Done.
echo   bin\Snag.exe       the app
echo   bin\SnagSetup.exe  the installer, share this one
exit /b 0

:npmfail
popd
echo Frontend step failed. Copy the errors above and send them over.
exit /b 1
