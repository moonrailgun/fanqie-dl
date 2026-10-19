#requires -Version 5.1
$ErrorActionPreference = 'Stop'
$gitExecPath = git --exec-path
if ($LASTEXITCODE -ne 0) { throw 'Cannot locate Git; install Git for Windows first' }
$bash = [IO.Path]::GetFullPath((Join-Path $gitExecPath '../../../bin/bash.exe'))
if (-not (Test-Path -LiteralPath $bash -PathType Leaf)) {
    throw 'Git Bash was not found; install the full Git for Windows distribution'
}
& $bash (Join-Path $PSScriptRoot 'release.sh') @args
exit $LASTEXITCODE
