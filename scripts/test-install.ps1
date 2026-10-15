#requires -Version 5.1
param(
    [string]$BinaryPath,
    [switch]$Child,
    [string]$FixtureDir,
    [string]$InstallDir,
    [string]$Scenario,
    [string]$Version = 'v1.2.3',
    [string]$Architecture = 'AMD64'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repoDir = Split-Path $PSScriptRoot -Parent

if ($Child) {
    # Mock transport and CPU detection in this child process only.
    $env:PROCESSOR_ARCHITEW6432 = ''
    $env:PROCESSOR_ARCHITECTURE = $Architecture
    function Invoke-RestMethod {
        param($Uri, $Headers, $TimeoutSec)
        if ($Uri -ne 'https://api.github.com/repos/longafterx/gocloc/releases/latest') {
            throw "Unexpected URL: $Uri"
        }
        Add-Content -LiteralPath (Join-Path $FixtureDir 'requests') -Value $Uri
        if ($Scenario -eq 'latest') { throw 'Mock HTTP 404' }
        return @{ tag_name = 'v1.2.3' }
    }
    function Invoke-WebRequest {
        param([switch]$UseBasicParsing, $Uri, $OutFile, $TimeoutSec)
        if ($Uri -notlike 'https://github.com/longafterx/gocloc/releases/download/v1.2.3/*') {
            throw "Unexpected URL: $Uri"
        }
        Add-Content -LiteralPath (Join-Path $FixtureDir 'requests') -Value $Uri
        if ($Scenario -eq 'download') { throw 'Mock HTTP 404' }
        $asset = ([uri]$Uri).Segments[-1]
        $sourceDir = $FixtureDir
        if ($Scenario -eq 'binary') { $sourceDir = Join-Path $FixtureDir 'invalid' }
        Copy-Item -LiteralPath (Join-Path $sourceDir $asset) -Destination $OutFile
        if ($asset -like '*.zip' -and $Scenario -eq 'corrupt') {
            Add-Content -LiteralPath $OutFile -Value 'corrupt'
        }
        if ($asset -like '*_checksums.txt') {
            switch ($Scenario) {
                'missing' { Set-Content -LiteralPath $OutFile -Value '' }
                'duplicate' { Get-Content -LiteralPath (Join-Path $sourceDir $asset) | Add-Content -LiteralPath $OutFile }
            }
        }
    }
    # Help returns without setting an exit code; failures must reach the parent.
    $LASTEXITCODE = 0
    & (Join-Path $repoDir 'install.ps1') -Version $Version -InstallDir $InstallDir -Help:($Scenario -eq 'help')
    exit $LASTEXITCODE
}

if (-not $BinaryPath) { throw 'Supply -BinaryPath pointing to a locally built Windows gocloc.exe' }
$BinaryPath = (Resolve-Path -LiteralPath $BinaryPath).Path
$testDir = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
$null = New-Item -ItemType Directory -Path $testDir
try {
    $fixtures = Join-Path $testDir 'releases'
    $invalidFixtures = Join-Path $fixtures 'invalid'
    $bundle = Join-Path $testDir 'bundle'
    $invalidBundle = Join-Path $testDir 'invalid-bundle'
    $destination = Join-Path $testDir 'bin with spaces'
    foreach ($directory in @($fixtures, $invalidFixtures, $bundle, $invalidBundle, $destination)) {
        $null = New-Item -ItemType Directory -Path $directory
    }
    Copy-Item -LiteralPath $BinaryPath -Destination (Join-Path $bundle 'gocloc.exe')
    Set-Content -LiteralPath (Join-Path $invalidBundle 'gocloc.exe') -Value 'not an executable'
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    foreach ($arch in @('x86_64', 'i386')) {
        $asset = "gocloc_Windows_$arch.zip"
        [IO.Compression.ZipFile]::CreateFromDirectory($bundle, (Join-Path $fixtures $asset))
        [IO.Compression.ZipFile]::CreateFromDirectory($invalidBundle, (Join-Path $invalidFixtures $asset))
        foreach ($directory in @($fixtures, $invalidFixtures)) {
            $hash = (Get-FileHash -LiteralPath (Join-Path $directory $asset) -Algorithm SHA256).Hash
            Add-Content -LiteralPath (Join-Path $directory 'gocloc_1.2.3_checksums.txt') -Value "$hash  $asset" -Encoding ASCII
        }
    }
    $engine = (Get-Process -Id $PID).Path
    $testScript = $PSCommandPath
    function Assert-Install {
        param(
            [string]$Name,
            [int]$ExpectedExit,
            [string]$ExpectedText,
            [string]$Failure = 'none',
            [string]$Tag = 'v1.2.3',
            [string]$Arch = 'AMD64',
            [string]$Target = $destination
        )
        # Keep expected native stderr failures from terminating PowerShell 5.1.
        $ErrorActionPreference = 'Continue'
        $output = & $engine -NoProfile -File $testScript -Child -FixtureDir $fixtures `
            -InstallDir $Target -Scenario $Failure -Version $Tag -Architecture $Arch 2>&1
        $status = $LASTEXITCODE
        $ErrorActionPreference = 'Stop'
        $message = $output | Out-String
        if ($status -ne $ExpectedExit -or -not $message.Contains($ExpectedText)) {
            throw "FAIL: $Name (exit $status, expected $ExpectedExit)`n$message"
        }
        Write-Output "PASS: $Name"
    }

    Assert-Install -Name help -ExpectedExit 0 -ExpectedText 'Usage:' -Failure help
    Assert-Install -Name invalid-version -ExpectedExit 1 -ExpectedText 'Invalid release version' -Tag '../bad'
    Assert-Install -Name latest -ExpectedExit 0 -ExpectedText 'Installed' -Tag latest
    $requests = Get-Content -LiteralPath (Join-Path $fixtures 'requests') -Raw
    if (-not $requests.Contains('/releases/latest')) { throw 'Latest release was not resolved' }
    $installedBinary = Join-Path $destination 'gocloc.exe'
    $originalHash = (Get-FileHash -LiteralPath $BinaryPath).Hash
    if ((Get-FileHash -LiteralPath $installedBinary).Hash -ne $originalHash) { throw 'Wrong installed binary' }
    foreach ($arch in @('AMD64', 'x86')) {
        Set-Content -LiteralPath (Join-Path $fixtures 'requests') -Value ''
        Assert-Install -Name "architecture-$arch" -ExpectedExit 0 -ExpectedText 'Installed' -Tag 1.2.3 -Arch $arch
        $requests = Get-Content -LiteralPath (Join-Path $fixtures 'requests') -Raw
        $assetArch = if ($arch -eq 'AMD64') { 'x86_64' } else { 'i386' }
        if (-not $requests.Contains("gocloc_Windows_$assetArch.zip") -or $requests.Contains('/releases/latest')) {
            throw "Wrong release lookup for pinned version / $arch"
        }
    }
    Assert-Install -Name unsupported-architecture -ExpectedExit 1 -ExpectedText 'No Windows release binary' -Arch ARM64
    foreach ($failure in @('latest', 'download', 'corrupt', 'missing', 'duplicate', 'binary')) {
        $expected = switch ($failure) {
            latest { 'Cannot find the latest release' }
            download { 'Cannot download' }
            corrupt { 'SHA-256 mismatch' }
            missing { 'Missing or ambiguous' }
            duplicate { 'Missing or ambiguous' }
            binary { 'Downloaded binary cannot run' }
        }
        Assert-Install -Name $failure -ExpectedExit 1 -ExpectedText $expected -Failure $failure -Tag latest
        if ((Get-FileHash -LiteralPath $installedBinary).Hash -ne $originalHash) { throw "Existing binary changed: $failure" }
        if (@(Get-ChildItem -LiteralPath $destination -Filter '.gocloc-*' -Force).Count -ne 0) {
            throw "Staged binary was not cleaned up: $failure"
        }
    }
    $directoryTarget = Join-Path $testDir 'directory-target'
    $null = New-Item -ItemType Directory -Path (Join-Path $directoryTarget 'gocloc.exe') -Force
    Assert-Install -Name directory-target -ExpectedExit 1 -ExpectedText 'Refusing to replace' -Target $directoryTarget
    Assert-Install -Name successful-upgrade -ExpectedExit 0 -ExpectedText 'Installed'
    Write-Output 'All Windows installer tests passed.'
} finally {
    Remove-Item -LiteralPath $testDir -Recurse -Force
}
