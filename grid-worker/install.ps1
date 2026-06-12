# Grid Worker Windows Installer
# Usage: iwr -useb https://example.com/install.ps1 | iex
# Or:    .\install.ps1 [-Version v1.0.0] [-InstallDir "C:\Program Files\grid-worker"]

param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:ProgramFiles\grid-worker"
)

$ErrorActionPreference = "Stop"
$Repo = "grid-computing/grid-worker"
$Binary = "grid-worker.exe"

function Write-Log {
    param([string]$Message, [string]$Level = "INFO")
    $color = switch ($Level) {
        "ERROR" { "Red" }
        "WARN"  { "Yellow" }
        default { "Green" }
    }
    Write-Host "[grid-worker] $Message" -ForegroundColor $color
}

function Get-LatestVersion {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    return $release.tag_name
}

function Get-Architecture {
    if ([System.Environment]::Is64BitOperatingSystem) {
        return "amd64"
    }
    throw "Only 64-bit Windows is supported"
}

function Verify-Checksum {
    param([string]$FilePath, [string]$ExpectedHash)
    $actual = (Get-FileHash -Path $FilePath -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $ExpectedHash.ToLower()) {
        throw "Checksum mismatch! Expected: $ExpectedHash, Got: $actual"
    }
}

try {
    # Resolve version
    if ($Version -eq "latest") {
        Write-Log "Resolving latest version..."
        $Version = Get-LatestVersion
    }

    $arch = Get-Architecture
    $VersionNum = $Version.TrimStart("v")
    $ArchiveName = "grid-worker_${VersionNum}_windows_${arch}.zip"
    $BaseUrl = "https://github.com/$Repo/releases/download/$Version"
    $TempDir = [System.IO.Path]::GetTempPath() + [System.Guid]::NewGuid().ToString()

    Write-Log "Installing grid-worker $Version for windows/$arch..."
    New-Item -ItemType Directory -Path $TempDir | Out-Null

    try {
        # Download archive
        $ArchivePath = Join-Path $TempDir $ArchiveName
        Write-Log "Downloading $ArchiveName..."
        Invoke-WebRequest -Uri "$BaseUrl/$ArchiveName" -OutFile $ArchivePath -UseBasicParsing

        # Download and verify checksums
        $ChecksumsName = "grid-worker_${VersionNum}_checksums.txt"
        $ChecksumsPath = Join-Path $TempDir $ChecksumsName
        try {
            Invoke-WebRequest -Uri "$BaseUrl/$ChecksumsName" -OutFile $ChecksumsPath -UseBasicParsing
            $checksums = Get-Content $ChecksumsPath
            $archiveHash = ($checksums | Where-Object { $_ -match $ArchiveName }).Split(" ")[0]
            if ($archiveHash) {
                Write-Log "Verifying checksum..."
                Verify-Checksum -FilePath $ArchivePath -ExpectedHash $archiveHash
                Write-Log "Checksum verified ✓"
            }
        } catch {
            Write-Log "Could not verify checksum: $_" -Level "WARN"
        }

        # Extract
        Expand-Archive -Path $ArchivePath -DestinationPath $TempDir -Force

        # Create install dir if needed
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }

        # Move binary
        $BinarySource = Join-Path $TempDir $Binary
        $BinaryDest = Join-Path $InstallDir $Binary
        Copy-Item -Path $BinarySource -Destination $BinaryDest -Force

        Write-Log "Installed to $BinaryDest"

        # Add to PATH if not already there
        $currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "Machine")
        if ($currentPath -notlike "*$InstallDir*") {
            [System.Environment]::SetEnvironmentVariable(
                "PATH",
                "$currentPath;$InstallDir",
                "Machine"
            )
            Write-Log "Added $InstallDir to system PATH"
            Write-Log "Restart your terminal for PATH changes to take effect"
        }

        Write-Log "Installation complete!"
        Write-Host ""
        Write-Host "Next steps:" -ForegroundColor Green
        Write-Host "  1. Configure:              grid-worker set-key <your-api-key>"
        Write-Host "  2. Run preflight checks:   grid-worker preflight"
        Write-Host "  3. Install as Windows Service: grid-worker install (as Administrator)"
        Write-Host "  4. Check status:           grid-worker status"
        Write-Host ""
        Write-Host "  Config file: $env:APPDATA\grid-worker\config.yaml"

    } finally {
        Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
    }

} catch {
    Write-Log "Installation failed: $_" -Level "ERROR"
    exit 1
}
