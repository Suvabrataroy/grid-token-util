@echo off
setlocal enabledelayedexpansion

REM ============================================================
REM  run-server.bat  —  Prerequisite check + start Grid Server
REM ============================================================

set "ROOT=%~dp0"
set "CP_DIR=%ROOT%control-plane"
set "BIN=%CP_DIR%\bin\control-plane.exe"
set "ERRORS=0"

echo [server] Checking prerequisites...
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
            REM Refresh PATH from registry
            for /f "tokens=2*" %%A in ('reg query "HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment" /v Path 2^>nul') do set "PATH=%%B;%PATH%"
        )
    ) else (
        echo [prereq] ERROR: winget not available. Install Go 1.22+ from https://go.dev/dl/
        set "ERRORS=1"
    )
) else (
    for /f "tokens=3" %%V in ('go version 2^>nul') do set "GOVER=%%V"
    echo [prereq] Go found: !GOVER!
    REM Extract major.minor (go1.22 → 1 22)
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

REM ── 3. PostgreSQL ─────────────────────────────────────────────────────────────
echo [prereq] Checking PostgreSQL...
sc query postgresql >nul 2>&1
if not errorlevel 1 (
    for /f "tokens=4" %%S in ('sc query postgresql 2^>nul ^| findstr "STATE"') do set "PG_STATE=%%S"
    if "!PG_STATE!"=="RUNNING" (
        echo [prereq] PostgreSQL service is running.
    ) else (
        echo [prereq] PostgreSQL service found but not running. Starting...
        net start postgresql >nul 2>&1
        if errorlevel 1 (
            REM Try common versioned service names
            set "PG_STARTED=0"
            for %%N in (postgresql-x64-16 postgresql-x64-15 postgresql-x64-14 "PostgreSQL 16" "PostgreSQL 15") do (
                if "!PG_STARTED!"=="0" (
                    net start "%%N" >nul 2>&1
                    if not errorlevel 1 set "PG_STARTED=1"
                )
            )
            if "!PG_STARTED!"=="0" (
                echo [prereq] WARNING: Could not start PostgreSQL service. Start it manually.
            ) else (
                echo [prereq] PostgreSQL service started.
            )
        ) else (
            echo [prereq] PostgreSQL service started.
        )
    )
) else (
    REM Try versioned service names (PostgreSQL 16, 15, 14)
    set "PG_FOUND=0"
    for %%N in (postgresql-x64-16 postgresql-x64-15 postgresql-x64-14) do (
        if "!PG_FOUND!"=="0" (
            sc query "%%N" >nul 2>&1
            if not errorlevel 1 (
                set "PG_FOUND=1"
                for /f "tokens=4" %%S in ('sc query "%%N" 2^>nul ^| findstr "STATE"') do set "PG_STATE=%%S"
                if "!PG_STATE!"=="RUNNING" (
                    echo [prereq] PostgreSQL service ^(%%N^) is running.
                ) else (
                    echo [prereq] Starting PostgreSQL service ^(%%N^)...
                    net start "%%N" >nul 2>&1
                    if errorlevel 1 (
                        echo [prereq] WARNING: Could not start %%N. Check services manually.
                    ) else (
                        echo [prereq] PostgreSQL service started.
                    )
                )
            )
        )
    )
    if "!PG_FOUND!"=="0" (
        REM Check if psql is at least on PATH (portable install)
        psql --version >nul 2>&1
        if not errorlevel 1 (
            echo [prereq] PostgreSQL binary found on PATH ^(portable or custom install^).
        ) else (
            echo [prereq] PostgreSQL not found.
            if "%HAS_WINGET%"=="1" (
                echo [prereq] Installing PostgreSQL 16 via winget...
                winget install PostgreSQL.PostgreSQL.16 --silent --accept-package-agreements --accept-source-agreements
                if errorlevel 1 (
                    echo [prereq] ERROR: winget install failed. Install PostgreSQL 16 from https://www.postgresql.org/download/windows/
                    set "ERRORS=1"
                ) else (
                    echo [prereq] PostgreSQL installed. Run the initialisation wizard then re-run this script.
                    set "ERRORS=1"
                )
            ) else (
                echo [prereq] ERROR: PostgreSQL not found. Install from https://www.postgresql.org/download/windows/
                set "ERRORS=1"
            )
        )
    )
)

REM ── 4. Redis ──────────────────────────────────────────────────────────────────
echo [prereq] Checking Redis...
sc query Redis >nul 2>&1
if not errorlevel 1 (
    for /f "tokens=4" %%S in ('sc query Redis 2^>nul ^| findstr "STATE"') do set "REDIS_STATE=%%S"
    if "!REDIS_STATE!"=="RUNNING" (
        echo [prereq] Redis service is running.
    ) else (
        echo [prereq] Redis service found but not running. Starting...
        net start Redis >nul 2>&1
        if errorlevel 1 (
            echo [prereq] WARNING: Could not start Redis service. Start it manually.
        ) else (
            echo [prereq] Redis service started.
        )
    )
) else (
    REM Check if redis-server is on PATH (portable/Memurai)
    redis-server --version >nul 2>&1
    if not errorlevel 1 (
        echo [prereq] Redis binary found on PATH.
        REM Check if already running via redis-cli ping
        redis-cli ping >nul 2>&1
        if not errorlevel 1 (
            echo [prereq] Redis is responding to PING.
        ) else (
            echo [prereq] WARNING: Redis binary found but not responding. Start redis-server manually.
        )
    ) else (
        echo [prereq] Redis not found.
        if "%HAS_WINGET%"=="1" (
            echo [prereq] Installing Memurai ^(Redis-compatible for Windows^) via winget...
            winget install Memurai.Memurai --silent --accept-package-agreements --accept-source-agreements
            if errorlevel 1 (
                echo [prereq] winget install of Memurai failed. Trying Redis for Windows...
                winget install tporadowski.redis --silent --accept-package-agreements --accept-source-agreements
                if errorlevel 1 (
                    echo [prereq] ERROR: Could not install Redis. Install Memurai from https://www.memurai.com/ or Redis from https://github.com/tporadowski/redis/releases
                    set "ERRORS=1"
                ) else (
                    echo [prereq] Redis installed. Restart this script to continue.
                    set "ERRORS=1"
                )
            ) else (
                echo [prereq] Memurai installed. Restart this script to continue.
                set "ERRORS=1"
            )
        ) else (
            echo [prereq] ERROR: Redis not found. Install Memurai from https://www.memurai.com/ or Redis from https://github.com/tporadowski/redis/releases
            set "ERRORS=1"
        )
    )
)

REM ── 5. Abort if any hard error ────────────────────────────────────────────────
echo.
if "%ERRORS%"=="1" (
    echo [server] One or more prerequisites are missing or could not be installed.
    echo [server] Please resolve the errors above, then re-run this script.
    pause
    exit /b 1
)

echo [server] All prerequisites satisfied.
echo.

REM ── 6. Build and run ─────────────────────────────────────────────────────────
echo [server] Building control-plane...
pushd "%CP_DIR%"

go build -o "%BIN%" .\cmd\server
if errorlevel 1 (
    echo [server] Build FAILED. Check Go errors above.
    popd
    pause
    exit /b 1
)

echo [server] Build OK — starting server...
echo [server] Press Ctrl+C to stop.
echo.

"%BIN%"

popd
endlocal
