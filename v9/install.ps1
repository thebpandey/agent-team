[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('codex', 'claude', 'both')]
    [string]$TargetHost,
    [string]$CodexHome,
    [string]$ClaudeHome
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Resolve-Home {
    param(
        [string]$ExplicitHome,
        [Parameter(Mandatory = $true)][string]$EnvironmentName,
        [Parameter(Mandatory = $true)][string]$DefaultLeaf
    )

    if (-not [string]::IsNullOrWhiteSpace($ExplicitHome)) {
        return [System.IO.Path]::GetFullPath($ExplicitHome)
    }

    $configuredHome = [System.Environment]::GetEnvironmentVariable($EnvironmentName)
    if (-not [string]::IsNullOrWhiteSpace($configuredHome)) {
        return [System.IO.Path]::GetFullPath($configuredHome)
    }

    $userProfile = [System.Environment]::GetFolderPath([System.Environment+SpecialFolder]::UserProfile)
    if ([string]::IsNullOrWhiteSpace($userProfile)) {
        throw "Unable to resolve the current user's home for $EnvironmentName."
    }

    return Join-Path $userProfile $DefaultLeaf
}

function Get-ExistingItem {
    param([Parameter(Mandatory = $true)][string]$Path)

    return Get-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
}

function Assert-NoReparsePath {
    param([Parameter(Mandatory = $true)][string]$Path)

    $candidate = [System.IO.Path]::GetFullPath($Path)
    while ($true) {
        $item = Get-ExistingItem $candidate
        if ($null -ne $item -and (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0)) {
            throw "Refusing reparse target path: $candidate"
        }

        $parent = Split-Path -Parent $candidate
        if ([string]::IsNullOrEmpty($parent) -or $parent -eq $candidate) {
            return
        }
        $candidate = $parent
    }
}

function Get-RelativePath {
    param(
        [Parameter(Mandatory = $true)][string]$Root,
        [Parameter(Mandatory = $true)][string]$Path
    )

    return $Path.Substring($Root.Length).TrimStart([char[]]'\\/')
}

function Copy-AndVerifyTree {
    param(
        [Parameter(Mandatory = $true)][string]$SourceRoot,
        [Parameter(Mandatory = $true)][string]$DestinationRoot
    )

    $sourceDirectories = @(Get-ChildItem -LiteralPath $SourceRoot -Directory -Recurse -Force)
    foreach ($directory in $sourceDirectories) {
        $relative = Get-RelativePath -Root $SourceRoot -Path $directory.FullName
        New-Item -ItemType Directory -Path (Join-Path $DestinationRoot $relative) -Force | Out-Null
    }

    $sourceFiles = @(Get-ChildItem -LiteralPath $SourceRoot -File -Recurse -Force)
    foreach ($file in $sourceFiles) {
        $relative = Get-RelativePath -Root $SourceRoot -Path $file.FullName
        $destination = Join-Path $DestinationRoot $relative
        $destinationParent = Split-Path -Parent $destination
        New-Item -ItemType Directory -Path $destinationParent -Force | Out-Null
        Copy-Item -LiteralPath $file.FullName -Destination $destination

        $sourceHash = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
        $destinationHash = (Get-FileHash -LiteralPath $destination -Algorithm SHA256).Hash
        if ($sourceHash -ne $destinationHash) {
            throw "Hash verification failed for $relative."
        }
    }
}

function Install-Skill {
    param(
        [Parameter(Mandatory = $true)][string]$Host,
        [Parameter(Mandatory = $true)][string]$Home,
        [Parameter(Mandatory = $true)][string]$SourceRoot
    )

    $targetRoot = Join-Path $Home 'skills/agent-team'
    Assert-NoReparsePath $targetRoot
    $targetItem = Get-ExistingItem $targetRoot
    if ($null -ne $targetItem -and -not $targetItem.PSIsContainer) {
        throw "Refusing non-directory skill target: $targetRoot"
    }

    $backupRoot = $null
    try {
        if ($null -ne $targetItem) {
            $backupRoot = Join-Path (Split-Path -Parent $targetRoot) ("agent-team.backup-" + [guid]::NewGuid().ToString('N'))
            Move-Item -LiteralPath $targetRoot -Destination $backupRoot
        }

        if ($env:AGENT_TEAM_INSTALL_TEST_FAIL_AFTER_BACKUP -eq '1') {
            throw 'Simulated failure after backup.'
        }

        New-Item -ItemType Directory -Path $targetRoot -Force | Out-Null
        Copy-AndVerifyTree -SourceRoot $SourceRoot -DestinationRoot $targetRoot
    }
    catch {
        $failure = $_
        try {
            $partialRoot = Get-ExistingItem $targetRoot
            if ($null -ne $partialRoot) {
                Assert-NoReparsePath $targetRoot
                Remove-Item -LiteralPath $targetRoot -Recurse -Force
            }
            if ($null -ne $backupRoot -and $null -ne (Get-ExistingItem $backupRoot)) {
                Move-Item -LiteralPath $backupRoot -Destination $targetRoot
                Write-Output "Restored $Host skill: $targetRoot"
            }
        }
        catch {
            throw "Install failed: $($failure.Exception.Message). Restore also failed: $($_.Exception.Message)"
        }
        throw $failure
    }

    Write-Output "Installed $Host skill: $targetRoot"
    if ($null -ne $backupRoot) {
        Write-Output "Backup for $Host: $backupRoot"
    }
    else {
        Write-Output "Backup for $Host: none"
    }
}

$sourceRoot = Join-Path $PSScriptRoot 'agent-team'
if (-not (Test-Path -LiteralPath $sourceRoot -PathType Container)) {
    throw "Missing packaged skill source: $sourceRoot"
}
if (-not (Test-Path -LiteralPath (Join-Path $sourceRoot 'SKILL.md') -PathType Leaf)) {
    throw "Missing packaged skill entrypoint: $(Join-Path $sourceRoot 'SKILL.md')"
}

if ($TargetHost -eq 'codex' -or $TargetHost -eq 'both') {
    Install-Skill -Host 'codex' -Home (Resolve-Home -ExplicitHome $CodexHome -EnvironmentName 'CODEX_HOME' -DefaultLeaf '.agents') -SourceRoot $sourceRoot
}
if ($TargetHost -eq 'claude' -or $TargetHost -eq 'both') {
    Install-Skill -Host 'claude' -Home (Resolve-Home -ExplicitHome $ClaudeHome -EnvironmentName 'CLAUDE_HOME' -DefaultLeaf '.claude') -SourceRoot $sourceRoot
}
