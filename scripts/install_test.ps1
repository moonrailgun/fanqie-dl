#requires -Version 5.1
$ErrorActionPreference = 'Stop'
$installer = Join-Path $PSScriptRoot 'install.ps1'
$fixtureDir = Join-Path ([IO.Path]::GetTempPath()) ('fanqie-install-test-' + [guid]::NewGuid())
$savedEnv = @{}
foreach ($name in 'OS', 'LOCALAPPDATA', 'FANQIE_DL_INSTALL_DIR', 'PROCESSOR_ARCHITECTURE', 'PROCESSOR_ARCHITEW6432', 'PATH') {
    $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$onWindows = [Environment]::OSVersion.Platform -eq 'Win32NT'
$global:fanqieInstallTest = @{ Requests = @(); Tag = 'v1.2.3'; NetworkFailure = $false }

function Assert($condition, $message) {
    if (-not $condition) { throw $message }
}
function Invoke-RestMethod {
    param($Uri, $TimeoutSec)
    Assert ($Uri -eq 'https://api.github.com/repos/moonrailgun/fanqie-dl/releases/latest') 'unexpected release URL'
    if ($global:fanqieInstallTest.NetworkFailure) { throw 'network failure' }
    return @{ tag_name = $global:fanqieInstallTest.Tag }
}
function Invoke-WebRequest {
    param($Uri, $OutFile, $TimeoutSec, [switch]$UseBasicParsing)
    $global:fanqieInstallTest.Requests += $Uri
    Assert ($Uri.StartsWith('https://github.com/moonrailgun/fanqie-dl/releases/download/v1.2.3/')) 'unexpected download URL'
    Copy-Item -LiteralPath (Join-Path $fixtureDir ($Uri.Split('/')[-1])) -Destination $OutFile
}
function Update-Checksums {
    $lines = Get-ChildItem -LiteralPath $fixtureDir -Filter '*.zip' | ForEach-Object {
        '{0}  {1}' -f (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash, $_.Name
    }
    Set-Content -LiteralPath (Join-Path $fixtureDir 'checksums.txt') -Value $lines
}
function Expect-Failure($message) {
    $failed = $false
    try { & $installer } catch {
        $failed = $true
        Assert ($_.Exception.Message -like "*$message*") "unexpected failure: $_"
    }
    Assert $failed "expected failure: $message"
    Assert ((Get-Content -LiteralPath $target -Raw) -eq 'old binary') 'old binary was changed'
}

try {
    $null = New-Item -ItemType Directory -Path $fixtureDir
    $payload = Join-Path $fixtureDir 'fanqie-dl.exe'
    [IO.File]::WriteAllText($payload, 'new binary')
    foreach ($arch in 'amd64', 'arm64') {
        Compress-Archive -LiteralPath $payload -DestinationPath (Join-Path $fixtureDir "fanqie-dl_1.2.3_windows_$arch.zip")
    }
    Update-Checksums
    $env:OS = 'Windows_NT'
    $env:LOCALAPPDATA = Join-Path $fixtureDir 'local app data'
    $env:FANQIE_DL_INSTALL_DIR = $null
    $env:PROCESSOR_ARCHITEW6432 = $null
    $env:PROCESSOR_ARCHITECTURE = 'AMD64'
    & $installer
    $defaultDir = Join-Path $env:LOCALAPPDATA 'Programs/fanqie-dl'
    $target = Join-Path $defaultDir 'fanqie-dl.exe'
    Assert ((Get-Content -LiteralPath $target -Raw) -eq 'new binary') 'default installation failed'
    Assert (($env:PATH -split ';') -contains $defaultDir) 'current PATH was not updated'
    if ($onWindows) {
        Assert (([Environment]::GetEnvironmentVariable('Path', 'User') -split ';') -contains $defaultDir) 'user PATH was not persisted'
    }
    $env:FANQIE_DL_INSTALL_DIR = Join-Path $fixtureDir 'custom install'
    $target = Join-Path $env:FANQIE_DL_INSTALL_DIR 'fanqie-dl.exe'
    foreach ($arch in 'AMD64', 'ARM64') {
        $env:PROCESSOR_ARCHITECTURE = $arch
        & $installer
        Assert ((Get-Content -LiteralPath $target -Raw) -eq 'new binary') 'custom installation or upgrade failed'
        Assert ($global:fanqieInstallTest.Requests[-2] -like "*_windows_$($arch.ToLower()).zip") 'wrong architecture'
    }
    $env:PROCESSOR_ARCHITECTURE = 'x86'
    $env:PROCESSOR_ARCHITEW6432 = 'AMD64'
    & $installer
    Assert ($global:fanqieInstallTest.Requests[-2] -like '*_windows_amd64.zip') 'WOW64 architecture was not detected'
    Assert (@(($env:PATH -split ';') | Where-Object { $_ -eq $env:FANQIE_DL_INSTALL_DIR }).Count -eq 1) 'duplicate PATH entry'
    [IO.File]::WriteAllText($target, 'old binary')

    $global:fanqieInstallTest.NetworkFailure = $true
    Expect-Failure 'network failure'
    $global:fanqieInstallTest.NetworkFailure = $false
    $global:fanqieInstallTest.Tag = 'v1.2.3/../../bad'
    Expect-Failure 'release version'
    $global:fanqieInstallTest.Tag = 'v1.2.3'
    Set-Content -LiteralPath (Join-Path $fixtureDir 'checksums.txt') -Value 'invalid'
    Expect-Failure 'checksum'
    Update-Checksums
    $archive = Join-Path $fixtureDir 'fanqie-dl_1.2.3_windows_amd64.zip'
    [IO.File]::WriteAllText($archive, 'corrupt archive')
    Expect-Failure 'checksum'
    Update-Checksums
    Expect-Failure 'archive'
    Remove-Item -LiteralPath $archive
    Compress-Archive -LiteralPath (Join-Path $fixtureDir 'checksums.txt') -DestinationPath $archive
    Update-Checksums
    Expect-Failure 'binary'
    Remove-Item -LiteralPath $archive
    Expect-Failure 'does not exist'
    $env:PROCESSOR_ARCHITEW6432 = $null
    Expect-Failure 'architecture'
    $env:OS = 'Linux'
    Expect-Failure 'Windows'
    Write-Host 'PowerShell install tests passed'
} finally {
    foreach ($name in $savedEnv.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnv[$name], 'Process')
    }
    if ($onWindows) { [Environment]::SetEnvironmentVariable('Path', $userPath, 'User') }
    if (Test-Path -LiteralPath $fixtureDir) { Remove-Item -LiteralPath $fixtureDir -Recurse -Force }
}
