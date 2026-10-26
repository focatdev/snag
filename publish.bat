@echo off
setlocal
cd /d "%~dp0"

rem usage: publish.bat https://github.com/you/snag.git
rem first run sets up git and pushes, later runs push whatever changed

where git >nul 2>nul
if errorlevel 1 (
  echo Git is not installed. Get it from https://git-scm.com/download/win
  exit /b 1
)

rem the remote file tool can't write .github, so those files arrive as _github
if exist _github (
  if not exist .github (
    ren _github .github
  ) else (
    xcopy /E /I /Q /Y _github .github >nul
    rmdir /S /Q _github
  )
)
if exist frontend\src\components\Rail.tsx del frontend\src\components\Rail.tsx

git config user.email >nul 2>nul
if errorlevel 1 (
  echo Tell git who you are first:
  echo   git config --global user.name "your name"
  echo   git config --global user.email "you@example.com"
  exit /b 1
)

if not exist .git (
  if "%~1"=="" (
    echo usage: publish.bat https://github.com/you/snag.git
    echo Make an empty repo on github.com first, no readme or license.
    exit /b 1
  )
  git init -b main
  git remote add origin %1
)

git add -A
git diff --cached --quiet
if errorlevel 1 (
  set MSG=%~2
  if "%~2"=="" set MSG=Update
  if not exist .git\refs\heads\main set MSG=Snag 0.1.0
  call git commit -m "%%MSG%%"
)
git push -u origin main
if errorlevel 1 (
  echo Push failed. If it asked for a password, sign in with the browser window git opens.
  exit /b 1
)

echo.
echo Pushed. To publish a release, tag it:
echo   git tag v0.1.0
echo   git push origin v0.1.0
exit /b 0
