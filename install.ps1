#requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = 'latest',
    [string]$InstallDir,
    [switch]$Help
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($Help) {
    Write-Output 'Usage: .\install.ps1 [-Version VERSION] [-InstallDir DIRECTORY]'
    Write-Output 'Defaults: latest stable release, %LOCALAPPDATA%\gocloc\bin.'
    Write-Output 'Versions may include or omit the v prefix. No Go toolchain is required.'
    return
}

$workDir = $null
$stagedBinary = $null
$oldSecurityProtocol = [Net.ServicePointManager]::SecurityProtocol
try {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw 'This script supports Windows only. On macOS/Linux, use install.sh.'
    }
    # Check the native architecture even in a 32-bit PowerShell process.
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($architecture) {
        'AMD64' { 'x86_64' }
        'x86' { 'i386' }
        default { throw "No Windows release binary for architecture: $architecture" }
    }
    if (-not $InstallDir) {
        if (-not $env:LOCALAPPDATA) { throw 'LOCALAPPDATA is unset; use -InstallDir' }
        $InstallDir = Join-Path $env:LOCALAPPDATA 'gocloc\bin'
    }
    $InstallDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($InstallDir)
    [Net.ServicePointManager]::SecurityProtocol = $oldSecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    if ($Version -eq 'latest') {
        try {
            $release = Invoke-RestMethod -Uri 'https://api.github.com/repos/longafterx/gocloc/releases/latest' `
                -Headers @{ 'User-Agent' = 'gocloc-installer' } -TimeoutSec 60
            $Version = $release.tag_name
        } catch {
            throw 'Cannot find the latest release. Check your network, GitHub API rate limit, and Releases; a Git tag alone has no binaries. You can also use -Version.'
        }
    }
    if ($Version -notmatch '^v') { $Version = "v$Version" }
    if ($Version -cnotmatch '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?\z') {
        throw "Invalid release version: $Version"
    }

    $archive = "gocloc_Windows_$arch.zip"
    $checksums = "gocloc_$($Version.Substring(1))_checksums.txt"
    $downloadUrl = "https://github.com/longafterx/gocloc/releases/download/$Version"
    $workDir = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
    $null = New-Item -ItemType Directory -Path $workDir
    Write-Output "Downloading gocloc $Version for Windows/$arch..."
    foreach ($asset in @($archive, $checksums)) {
        try {
            Invoke-WebRequest -UseBasicParsing -Uri "$downloadUrl/$asset" `
                -OutFile (Join-Path $workDir $asset) -TimeoutSec 300
        } catch {
            throw "Cannot download $asset for $Version. Check the release assets and your network."
        }
    }
    $checksumPattern = '^([0-9a-fA-F]{64})\s+\*?' + [regex]::Escape($archive) + '$'
    $entries = @(Get-Content -LiteralPath (Join-Path $workDir $checksums) | Where-Object { $_ -match $checksumPattern })
    if ($entries.Count -ne 1) { throw "Missing or ambiguous SHA-256 entry for $archive" }
    $expected = [regex]::Match($entries[0], $checksumPattern).Groups[1].Value
    $actual = (Get-FileHash -LiteralPath (Join-Path $workDir $archive) -Algorithm SHA256).Hash
    if ($actual -ne $expected) { throw 'SHA-256 mismatch; the existing installation has not been changed' }

    $null = New-Item -ItemType Directory -Path $InstallDir -Force
    $destination = Join-Path $InstallDir 'gocloc.exe'
    if (Test-Path -LiteralPath $destination) {
        $item = Get-Item -LiteralPath $destination -Force
        if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) {
            throw "Refusing to replace a directory or symlink: $destination"
        }
    }
    $stagedBinary = Join-Path $InstallDir ('.gocloc-' + [guid]::NewGuid().ToString('N') + '.exe')
    # Extract only the executable, avoiding other paths from the ZIP archive.
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $workDir $archive))
    try {
        $entries = @($zip.Entries | Where-Object { $_.FullName -eq 'gocloc.exe' })
        if ($entries.Count -ne 1) { throw 'Archive must contain exactly one gocloc.exe' }
        [IO.Compression.ZipFileExtensions]::ExtractToFile($entries[0], $stagedBinary)
    } finally {
        $zip.Dispose()
    }
    try {
        $installedVersion = & $stagedBinary --version
    } catch {
        throw 'Downloaded binary cannot run on this machine'
    }
    if ($LASTEXITCODE -ne 0) { throw 'Downloaded binary cannot run on this machine' }
    if (Test-Path -LiteralPath $destination) {
        # Replace keeps the old binary intact if replacement fails (e.g. a file lock).
        # PowerShell 5.1 converts $null to an empty string for .NET string arguments.
        [IO.File]::Replace($stagedBinary, $destination, [NullString]::Value)
    } else {
        [IO.File]::Move($stagedBinary, $destination)
    }
    $stagedBinary = $null
    Write-Output "Installed $destination"
    Write-Output $installedVersion
    if (($env:PATH -split ';') -notcontains $InstallDir) {
        Write-Output "Add $InstallDir to the beginning of your user PATH, then open a new terminal."
    }
    $existing = Get-Command gocloc -ErrorAction SilentlyContinue
    if ($existing -and $existing.Source -ne $destination) {
        Write-Output "Note: your PATH currently selects $($existing.Source) instead."
    }
} catch {
    Write-Error $_ -ErrorAction Continue
    exit 1
} finally {
    [Net.ServicePointManager]::SecurityProtocol = $oldSecurityProtocol
    if ($stagedBinary -and (Test-Path -LiteralPath $stagedBinary)) {
        Remove-Item -LiteralPath $stagedBinary -Force
    }
    if ($workDir -and (Test-Path -LiteralPath $workDir)) {
        Remove-Item -LiteralPath $workDir -Recurse -Force
    }
}
