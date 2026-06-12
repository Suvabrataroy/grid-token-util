@echo off
setlocal enabledelayedexpansion

REM ============================================================
REM  run-worker.bat  —  Prerequisite check + start Grid Worker
REM ============================================================

set "ROOT=%~dp0"
set "GW_DIR=%ROOT%grid-worker"
set "BIN=%GW_DIR%\bin\grid-worker.exe"
set "ERRORS=0"
set "WARNINGS=0"

echo [worker] Checking prerequisites...
echo.

REM ── 1. Check for winget (used for auto-install) ──────────────────────────────
set "HAS_WINGET=0"
winget --version >nul 2>&1
if not errorlevel 1 set "HAS_WINGET=1"

REM ── 2. Go 1.22+ ──────────────────────────────────────────────────────────────
echo [prereq] Checking Go...
go version >nul 2>&1
if errorlevel 1 (
    echo [prereq] Go not found.
    if "%HAS_WINGET%"=="1" (
        echo [prereq] Installing Go via winget...
        winget install GoLang.Go --silent --accept-package-agreements --accept-source-agreements
        if errorlevel 1 (
            echo [prereq] ERROR: winget install failed. Install Go manually from https://go.dev/dl/
            set "ERRORS=1"
        ) else (
            echo [prereq] Go installed. You may need to restart this terminal for PATH to update.
            for /f "tokens=2*" %%A in ('reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v Path 2^>nul') do set "PATH=%%B;%PATH%"
        )
    ) else (
        echo [prereq] ERROR: winget not available. Install Go 1.22+ from https://go.dev/dl/
        set "ERRORS=1"
    )
) else (
    for /f "tokens=3" %%V in ('go version 2^>nul') do set "GOVER=%%V"
    echo [prereq] Go found: !GOVER!
    set "GOVER_NUM=!GOVER:go=!"
    for /f "tokens=1,2 delims=." %%A in ("!GOVER_NUM!") do (
        set "GO_MAJOR=%%A"
        set "GO_MINOR=%%B"
    )
    if !GO_MAJOR! LSS 1 (
        echo [prereq] ERROR: Go 1.22+ required, found !GOVER!
        set "ERRORS=1"
    ) else if !GO_MAJOR! EQU 1 if !GO_MINOR! LSS 22 (
        echo [prereq] ERROR: Go 1.22+ required, found !GOVER!
        set "ERRORS=1"
    ) else (
        echo [prereq] Go version OK.
    )
)

REM ── 3. Git 2.30+ ─────────────────────────────────────────────────────────────
echo [prereq] Checking Git...
git --version >nul 2>&1
if errorlevel 1 (
    echo [prereq] Git not found.
    if "%HAS_WINGET%"=="1" (
        echo [prereq] Installing Git via winget...
        winget install Git.Git --silent --accept-package-agreements --accept-source-agreements
        if errorlevel 1 (
            echo [prereq] ERROR: winget install failed. Install Git from https://git-scm.com/download/win
            set "ERRORS=1"
        ) else (
            echo [prereq] Git installed. You may need to restart this terminal for PATH to update.
            for /f "tokens=2*" %%A in ('reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v Path 2^>nul') do set "PATH=%%B;%PATH%"
        )
    ) else (
        echo [prereq] ERROR: Git not found. Install from https://git-scm.com/download/win
        set "ERRORS=1"
    )
) else (
    for /f "tokens=3" %%V in ('git --version 2^>nul') do set "GITVER=%%V"
    echo [prereq] Git found: !GITVER!
    for /f "tokens=1,2 delims=." %%A in ("!GITVER!") do (
        set "GIT_MAJOR=%%A"
        set "GIT_MINOR=%%B"
    )
    if !GIT_MAJOR! LSS 2 (
        echo [prereq] WARNING: Git 2.30+ recommended, found !GITVER!
        set "WARNINGS=1"
    ) else if !GIT_MAJOR! EQU 2 if !GIT_MINOR! LSS 30 (
        echo [prereq] WARNING: Git 2.30+ recommended, found !GITVER!
        set "WARNINGS=1"
    ) else (
        echo [prereq] Git version OK.
    )
)

REM ── 4. AI Agent binaries (at least one required) ─────────────────────────────
echo [prereq] Checking AI agent binaries...
set "AGENT_FOUND=0"

REM Claude (claude.exe)
claude --version >nul 2>&1
if not errorlevel 1 (
    echo [prereq] claude: found
    set "AGENT_FOUND=1"
) else (
    echo [prereq] claude: not on PATH
)

REM GitHub Copilot CLI (gh copilot or github-copilot-cli)
gh copilot --version >nul 2>&1
if not errorlevel 1 (
    echo [prereq] gh copilot: found
    set "AGENT_FOUND=1"
) else (
    github-copilot-cli --version >nul 2>&1
    if not errorlevel 1 (
        echo [prereq] github-copilot-cli: found
        set "AGENT_FOUND=1"
    ) else (
        echo [prereq] gh copilot / github-copilot-cli: not on PATH
    )
)

REM Gemini CLI (gemini.exe)
gemini --version >nul 2>&1
if not errorlevel 1 (
    echo [prereq] gemini: found
    set "AGENT_FOUND=1"
) else (
    echo [prereq] gemini: not on PATH
)

REM ChatGPT / openai CLI
openai --version >nul 2>&1
if not errorlevel 1 (
    echo [prereq] openai cli: found
    set "AGENT_FOUND=1"
) else (
    chatgpt --version >nul 2>&1
    if not errorlevel 1 (
        echo [prereq] chatgpt: found
        set "AGENT_FOUND=1"
    ) else (
        echo [prereq] openai/chatgpt cli: not on PATH
    )
)

if "%AGENT_FOUND%"=="0" (
    echo.
    echo [prereq] WARNING: No known AI agent binary was found on PATH.
    echo [prereq]   The worker will start but will fail to execute tasks until an agent is installed.
    echo [prereq]   Supported agents:
    echo [prereq]     - Claude Code:   https://claude.ai/code
    echo [prereq]     - GitHub Copilot: gh extension install github/gh-copilot
    echo [prereq]     - Gemini CLI:    https://ai.google.dev/gemini-api/docs/gemini-cli
    echo [prereq]     - OpenAI CLI:    pip install openai-cli
    echo.
    set "WARNINGS=1"
) else (
    echo [prereq] At least one AI agent found — OK.
)

REM ── 5. Worker config file ────────────────────────────────────────────────────
echo [prereq] Checking worker config...
set "CONFIG_FOUND=0"

REM Common config locations (in order of preference)
for %%P in (
    "%GW_DIR%\config.yaml"
    "%GW_DIR%\config.yml"
    "%APPDATA%\grid-worker\config.yaml"
    "%APPDATA%\grid-worker\config.yml"
    "%USERPROFILE%\.grid-worker\config.yaml"
    "%USERPROFILE%\.grid-worker\config.yml"
) do (
    if "!CONFIG_FOUND!"=="0" (
        if exist %%P (
            echo [prereq] Config file found: %%P
            set "CONFIG_FOUND=1"
        )
    )
)

if "!CONFIG_FOUND!"=="0" (
    echo [prereq] WARNING: No config file found.
    echo [prereq]   Expected locations:
    echo [prereq]     %GW_DIR%\config.yaml
    echo [prereq]     %APPDATA%\grid-worker\config.yaml
    echo [prereq]     %USERPROFILE%\.grid-worker\config.yaml
    echo [prereq]   The worker may fail to start without a valid config.
    echo [prereq]   See grid-worker\config.example.yaml for a template.
    set "WARNINGS=1"
)

REM ── 6. Abort on hard errors ───────────────────────────────────────────────────
echo.
if "%ERRORS%"=="1" (
    echo [worker] One or more prerequisites are missing or could not be installed.
    echo [worker] Please resolve the errors above, then re-run this script.
    pause
    exit /b 1
)

if "%WARNINGS%"=="1" (
    echo [worker] Prerequisites OK with warnings ^(see above^).
) else (
    echo [worker] All prerequisites satisfied.
)
echo.

REM ── 7. Build and run ─────────────────────────────────────────────────────────
echo [worker] Building grid-worker...
pushd "%GW_DIR%"

go build -o "%BIN%" .\cmd\grid-worker
if errorlevel 1 (
    echo [worker] Build FAILED. Check Go errors above.
    popd
    pause
    exit /b 1
)

echo [worker] Build OK — starting worker daemon...
echo [worker] Press Ctrl+C to stop.
echo.

"%BIN%" run

popd
endlocal
