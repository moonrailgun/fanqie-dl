#requires -Version 5.1
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$tmp = Join-Path ([IO.Path]::GetTempPath()) ('fanqie-release-test-' + [guid]::NewGuid())
$gitCommand = (Get-Command git -CommandType Application -TotalCount 1).Source
$savedEnv = @{}
foreach ($name in 'GIT_CONFIG_GLOBAL', 'GIT_CONFIG_NOSYSTEM', 'GIT_AUTHOR_NAME', 'GIT_AUTHOR_EMAIL', 'GIT_COMMITTER_NAME', 'GIT_COMMITTER_EMAIL') {
    $savedEnv[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
}
function Assert($condition, $message) {
    if (-not $condition) { throw $message }
}
function Run-Git {
    & $gitCommand @args
    if ($LASTEXITCODE -ne 0) { throw "git failed: $args" }
}
try {
    $null = New-Item -ItemType Directory -Path $tmp
    $gitConfig = Join-Path $tmp 'gitconfig'
    [IO.File]::WriteAllText($gitConfig, '')
    $env:GIT_CONFIG_GLOBAL = $gitConfig
    $env:GIT_CONFIG_NOSYSTEM = '1'
    $env:GIT_AUTHOR_NAME = $env:GIT_COMMITTER_NAME = 'Test'
    $env:GIT_AUTHOR_EMAIL = $env:GIT_COMMITTER_EMAIL = 'test@example.invalid'
    Run-Git init -q --bare (Join-Path $tmp 'remote.git')
    $repo = Join-Path $tmp 'repo with spaces'
    Run-Git init -q -b main $repo
    Copy-Item -LiteralPath (Join-Path $root 'release.ps1'), (Join-Path $root 'release.sh') -Destination $repo
    Push-Location $repo
    try {
        Run-Git config core.autocrlf false
        Run-Git add release.ps1 release.sh
        Run-Git commit -qm initial
        Run-Git remote add origin (Join-Path $tmp 'remote.git')
        Run-Git push -q -u origin main
        if ([Environment]::OSVersion.Platform -ne 'Win32NT') {
            # Exercise the Windows Git layout with native Bash on other test hosts.
            $global:fanqieReleaseExecPath = Join-Path $tmp 'Git/mingw64/libexec/git-core'
            $null = New-Item -ItemType Directory -Path $global:fanqieReleaseExecPath, (Join-Path $tmp 'Git/bin')
            Copy-Item -LiteralPath (Get-Command bash -CommandType Application -TotalCount 1).Source -Destination (Join-Path $tmp 'Git/bin/bash.exe')
            function git { $global:LASTEXITCODE = 0; $global:fanqieReleaseExecPath }
        }
        $wrapper = Join-Path $repo 'release.ps1'
        $output = & $wrapper patch --dry-run
        Assert ($LASTEXITCODE -eq 0) 'dry run failed'
        Assert ($output -match 'v0.1.0') 'wrong initial version'
        foreach ($target in '1.0.0', 'v1.0.0') {
            $output = & $wrapper $target --dry-run
            Assert ($LASTEXITCODE -eq 0) 'custom version dry run failed'
            Assert ($output -match 'none -> v1.0.0') 'wrapper did not forward the target version'
        }
        Assert (-not (Run-Git tag --list)) 'dry run created a tag'
        & $wrapper patch
        Assert ($LASTEXITCODE -eq 0) 'release failed'
        Assert ((Run-Git --git-dir (Join-Path $tmp 'remote.git') tag --list) -eq 'v0.1.0') 'tag was not pushed'
        & $wrapper patch
        Assert ($LASTEXITCODE -ne 0) 'wrapper lost the failure exit code'
        Run-Git commit --allow-empty -qm next
        Run-Git push -q origin main
        $output = & $wrapper minor --dry-run
        Assert ($LASTEXITCODE -eq 0) 'minor dry run failed'
        Assert ($output -match 'v0.1.0 -> v0.2.0') 'wrapper did not forward arguments'
        if ([Environment]::OSVersion.Platform -eq 'Win32NT') {
            $inputFile = Join-Path $tmp 'selection'
            $outputFile = Join-Path $tmp 'output'
            [IO.File]::WriteAllText($inputFile, "2`n")
            $process = Start-Process -FilePath (Get-Process -Id $PID).Path -ArgumentList @(
                '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', ('"' + $wrapper + '"'), '--dry-run'
            ) -RedirectStandardInput $inputFile -RedirectStandardOutput $outputFile -RedirectStandardError (Join-Path $tmp 'errors') -Wait -PassThru
            Assert ($process.ExitCode -eq 0) 'interactive release failed'
            Assert ((Get-Content -LiteralPath $outputFile -Raw) -match 'v0.1.0 -> v0.2.0') 'interactive selection was lost'
            Assert ((Run-Git tag --list) -eq 'v0.1.0') 'interactive dry run created a tag'
        }
        Write-Host 'PowerShell release tests passed'
    } finally { Pop-Location }
} finally {
    foreach ($name in $savedEnv.Keys) {
        [Environment]::SetEnvironmentVariable($name, $savedEnv[$name], 'Process')
    }
    if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Recurse -Force }
}
