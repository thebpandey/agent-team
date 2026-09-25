[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$installer = Join-Path $repoRoot 'install.ps1'
$testRoot = Join-Path $env:TEMP ([guid]::NewGuid().ToString('N'))

function Assert-True {
    param(
        [Parameter(Mandatory = $true)][bool]$Condition,
        [Parameter(Mandatory = $true)][string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Assert-SameTree {
    param(
        [Parameter(Mandatory = $true)][string]$ExpectedRoot,
        [Parameter(Mandatory = $true)][string]$ActualRoot
    )

    $expected = @(Get-ChildItem -LiteralPath $ExpectedRoot -File -Recurse -Force | ForEach-Object {
        $relative = $_.FullName.Substring($ExpectedRoot.Length).TrimStart([char[]]'\\/')
        "$relative`t$((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash)"
    } | Sort-Object)
    $actual = @(Get-ChildItem -LiteralPath $ActualRoot -File -Recurse -Force | ForEach-Object {
        $relative = $_.FullName.Substring($ActualRoot.Length).TrimStart([char[]]'\\/')
        "$relative`t$((Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash)"
    } | Sort-Object)

    $difference = Compare-Object -ReferenceObject $expected -DifferenceObject $actual
    Assert-True ($null -eq $difference) "installed tree differs from source: $($difference | Out-String)"
}

function Invoke-InstallerFailure {
    param([Parameter(Mandatory = $true)][hashtable]$Arguments)

    try {
        & $installer @Arguments
    }
    catch {
        return $_
    }

    throw 'installer unexpectedly succeeded'
}

try {
    $sourceRoot = Join-Path $repoRoot 'agent-team'
    $codexHome = Join-Path $testRoot 'codex'
    $claudeHome = Join-Path $testRoot 'claude'

    & $installer -TargetHost codex -CodexHome $codexHome
    $codexRoot = Join-Path $codexHome 'skills/agent-team'
    Assert-True (Test-Path -LiteralPath (Join-Path $codexRoot 'SKILL.md') -PathType Leaf) 'Codex skill missing'
    Assert-SameTree -ExpectedRoot $sourceRoot -ActualRoot $codexRoot

    & $installer -TargetHost both -CodexHome (Join-Path $testRoot 'both-codex') -ClaudeHome $claudeHome
    Assert-SameTree -ExpectedRoot $sourceRoot -ActualRoot (Join-Path $testRoot 'both-codex/skills/agent-team')
    Assert-SameTree -ExpectedRoot $sourceRoot -ActualRoot (Join-Path $claudeHome 'skills/agent-team')

    $unknownPath = Join-Path $codexRoot 'user-notes.bin'
    $unknownBytes = [byte[]](0, 1, 2, 253, 254, 255)
    [System.IO.File]::WriteAllBytes($unknownPath, $unknownBytes)
    $updateOutput = @(& $installer -TargetHost codex -CodexHome $codexHome)
    $backupLine = @($updateOutput | Where-Object { $_ -match '^Backup for codex: ' })[-1]
    Assert-True ($null -ne $backupLine) 'update did not print a Codex backup path'
    $backupRoot = $backupLine.ToString().Substring('Backup for codex: '.Length)
    $backupUnknown = [System.IO.File]::ReadAllBytes((Join-Path $backupRoot 'user-notes.bin'))
    Assert-True ([System.Linq.Enumerable]::SequenceEqual[byte]($unknownBytes, $backupUnknown)) 'backup did not preserve unknown bytes'
    Assert-True (-not (Test-Path -LiteralPath $unknownPath)) 'new install merged an unknown file'
    Assert-SameTree -ExpectedRoot $sourceRoot -ActualRoot $codexRoot

    $reparseHome = Join-Path $testRoot 'reparse-codex'
    $reparseSkills = Join-Path $reparseHome 'skills'
    $reparseElsewhere = Join-Path $testRoot 'reparse-target'
    New-Item -ItemType Directory -Path $reparseSkills -Force | Out-Null
    New-Item -ItemType Directory -Path $reparseElsewhere -Force | Out-Null
    New-Item -ItemType Junction -Path (Join-Path $reparseSkills 'agent-team') -Target $reparseElsewhere | Out-Null
    $null = Invoke-InstallerFailure -Arguments @{ TargetHost = 'codex'; CodexHome = $reparseHome }
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $reparseElsewhere 'SKILL.md'))) 'installer followed a reparse target'

    $restoreBytes = [byte[]](9, 8, 7, 6)
    $restoreUnknown = Join-Path $codexRoot 'restore-me.bin'
    [System.IO.File]::WriteAllBytes($restoreUnknown, $restoreBytes)
    $previousFaultSetting = $env:AGENT_TEAM_INSTALL_TEST_FAIL_AFTER_BACKUP
    try {
        $env:AGENT_TEAM_INSTALL_TEST_FAIL_AFTER_BACKUP = '1'
        $null = Invoke-InstallerFailure -Arguments @{ TargetHost = 'codex'; CodexHome = $codexHome }
    }
    finally {
        if ($null -eq $previousFaultSetting) {
            Remove-Item Env:AGENT_TEAM_INSTALL_TEST_FAIL_AFTER_BACKUP -ErrorAction SilentlyContinue
        }
        else {
            $env:AGENT_TEAM_INSTALL_TEST_FAIL_AFTER_BACKUP = $previousFaultSetting
        }
    }
    $restoredBytes = [System.IO.File]::ReadAllBytes($restoreUnknown)
    Assert-True ([System.Linq.Enumerable]::SequenceEqual[byte]($restoreBytes, $restoredBytes)) 'failed install did not restore the prior root'

    Write-Output 'PASS: Windows installer checks'
}
finally {
    if (Test-Path -LiteralPath $testRoot) {
        Remove-Item -LiteralPath $testRoot -Recurse -Force
    }
}
