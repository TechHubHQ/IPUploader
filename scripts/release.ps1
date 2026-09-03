<#
.SYNOPSIS
  Builds uploader.exe, tags a release, and publishes it to GitHub.
.DESCRIPTION
  Automates: build exe -> bump tag version -> create git tag -> push tag ->
  create GitHub release -> upload exe as release asset.

  Tag format: IPUploaderV<N>_<yyyy_MM_dd> (N auto-incremented from the highest existing tag).
.PARAMETER TagPrefix
  Prefix used when auto-generating the next tag. Default: IPUploaderV
.PARAMETER ReleaseNotes
  Body text for the GitHub release. Defaults to "Release <tag>".
.PARAMETER Draft
  Create the release as a draft.
.PARAMETER Tag
  Use this exact tag instead of auto-incrementing from TagPrefix.
.PARAMETER Force
  Replace an existing tag/release of the same name (local + remote tag, and the GitHub release + assets).
.PARAMETER ExeName
  Name of the built executable and release asset. Default: uploader.exe
.PARAMETER Help
  Show this usage help and exit.
.EXAMPLE
  .\release.ps1
.EXAMPLE
  .\release.ps1 -Tag IPUploaderV5_2026_09_03 -Force
.EXAMPLE
  .\release.ps1 -ExeName ipuploader.exe
#>
# Requires -Version 5.1
param(
  [string]$TagPrefix = "IPUploaderV",
  [string]$ReleaseNotes = "",
  [switch]$Draft,
  [string]$Tag,
  [switch]$Force,
  [string]$ExeName = "uploader.exe",
  [Alias("h")][switch]$Help
)

if ($Help) {
  Get-Help $PSCommandPath -Detailed
  exit 0
}

$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)

function Get-GitHubToken {
  $credOutput = "protocol=https`nhost=github.com`n" | git credential fill
  $match = $credOutput | Select-String "^password=(.*)$"
  if (-not $match) { throw "Could not resolve a GitHub credential via 'git credential fill'." }
  return $match.Matches[0].Groups[1].Value
}

function Get-RepoOwnerAndName {
  param($Headers)
  $url = git config --get remote.origin.url
  if ($url -match "github\.com[:/](?<owner>[^/]+)/(?<repo>[^/.]+)(\.git)?$") {
    $owner = $Matches.owner
    $repo = $Matches.repo
    # Repos can be renamed/transferred on GitHub; GET requests follow the redirect but
    # write requests (used later to create the release) do not, so resolve the canonical
    # owner/repo up front and fail fast if the local remote is stale.
    try {
      $info = Invoke-RestMethod -Uri "https://api.github.com/repos/$owner/$repo" -Headers $Headers
      if ($info.full_name -and $info.full_name -ne "$owner/$repo") {
        Write-Host "Remote 'origin' points to a renamed/moved repo ($owner/$repo -> $($info.full_name)); updating it."
        Invoke-Git @('remote', 'set-url', 'origin', $info.clone_url)
      }
      return @{ Owner = $info.owner.login; Repo = $info.name }
    } catch {
      throw "Could not resolve repo $owner/$repo via GitHub API: $($_.Exception.Message)"
    }
  }
  throw "Could not parse owner/repo from remote origin url: $url"
}

function Get-NextTag {
  param([string]$Prefix)
  $existing = git tag --list "$Prefix*"
  $maxVersion = 0
  foreach ($t in $existing) {
    if ($t -match "^$Prefix(\d+)_") {
      $v = [int]$Matches[1]
      if ($v -gt $maxVersion) { $maxVersion = $v }
    }
  }
  $next = $maxVersion + 1
  $date = Get-Date -Format "yyyy_MM_dd"
  return "$Prefix${next}_$date"
}

function Get-ReleaseByTag {
  param($Owner, $Repo, $TagName, $Headers)
  try {
    return Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases/tags/$TagName" -Headers $Headers
  } catch {
    if ($_.Exception.Response -and $_.Exception.Response.StatusCode.value__ -eq 404) { return $null }
    throw
  }
}

# git writes routine progress info (e.g. push summaries) to stderr even on success; running it
# with ErrorActionPreference=Continue keeps that from being treated as a terminating error.
function Invoke-Git {
  param([string[]]$GitArgs, [switch]$BestEffort)
  $prev = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  & git @GitArgs 2>&1 | ForEach-Object { Write-Host $_ }
  $ErrorActionPreference = $prev
  if ($LASTEXITCODE -ne 0 -and -not $BestEffort) {
    throw "git $($GitArgs -join ' ') failed with exit code $LASTEXITCODE"
  }
}

Write-Host "Building $ExeName..."
go build -o $ExeName .\cmd\uploader\
if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }

$repoInfo = Get-RepoOwnerAndName
$newTag = if ($Tag) { $Tag } else { Get-NextTag -Prefix $TagPrefix }
Write-Host "Target tag: $newTag"

$token = Get-GitHubToken
$headers = @{ Authorization = "token $token"; Accept = "application/vnd.github+json" }
$repoInfo = Get-RepoOwnerAndName -Headers $headers

$existingRelease = Get-ReleaseByTag -Owner $repoInfo.Owner -Repo $repoInfo.Repo -TagName $newTag -Headers $headers
$tagExistsLocally = [bool](git rev-parse -q --verify "refs/tags/$newTag" 2>$null)

if (($existingRelease -or $tagExistsLocally) -and -not $Force) {
  throw "Tag/release $newTag already exists. Re-run with -Force to replace it."
}

if ($Force) {
  if ($existingRelease) {
    Write-Host "Deleting existing release $newTag (id=$($existingRelease.id))..."
    Invoke-RestMethod -Uri "https://api.github.com/repos/$($repoInfo.Owner)/$($repoInfo.Repo)/releases/$($existingRelease.id)" -Method Delete -Headers $headers | Out-Null
  }
  if ($tagExistsLocally) {
    Write-Host "Deleting existing tag $newTag (local + remote)..."
    Invoke-Git @('tag', '-d', $newTag)
  }
  Invoke-Git -BestEffort @('push', 'origin', ":refs/tags/$newTag")
}

# Nothing below is "committed" until everything succeeds: the tag push, release, and asset
# upload are all rolled back on any failure so a partial run never leaves a broken release behind.
$tagPushed = $false
$releaseCreated = $false
try {
  Invoke-Git @('tag', '-a', $newTag, '-m', "Release $newTag")
  Invoke-Git @('push', 'origin', $newTag)
  $tagPushed = $true

  $body = @{
    tag_name = $newTag
    name = $newTag
    body = if ($ReleaseNotes) { $ReleaseNotes } else { "Release $newTag" }
    draft = [bool]$Draft
    prerelease = $false
  } | ConvertTo-Json

  Write-Host "Creating GitHub release..."
  $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$($repoInfo.Owner)/$($repoInfo.Repo)/releases" -Method Post -Headers $headers -Body $body -ContentType "application/json"
  $releaseCreated = $true
  Write-Host "Release created: $($release.html_url)"

  $assetOut = Join-Path $env:TEMP "uploader_asset_result.json"
  $uploadUri = "https://uploads.github.com/repos/$($repoInfo.Owner)/$($repoInfo.Repo)/releases/$($release.id)/assets?name=$ExeName"

  Write-Host "Uploading $ExeName asset (this can take a few minutes)..."
  & curl.exe -s -X POST -H "Authorization: token $token" -H "Content-Type: application/octet-stream" --data-binary "@$ExeName" $uploadUri -o $assetOut -w "HTTP_STATUS:%{http_code}`n"

  $asset = Get-Content $assetOut -Raw | ConvertFrom-Json
  Remove-Item $assetOut -ErrorAction SilentlyContinue

  if (-not $asset.browser_download_url) {
    throw "Asset upload failed: $($asset | ConvertTo-Json -Depth 5)"
  }

  Write-Host "Asset uploaded: $($asset.browser_download_url)"
  Write-Host "Done. Release: $($release.html_url)"
} catch {
  Write-Host "Release failed, rolling back any partial changes: $($_.Exception.Message)"
  if ($releaseCreated) {
    Write-Host "Deleting partially-created release $newTag (id=$($release.id))..."
    Invoke-RestMethod -Uri "https://api.github.com/repos/$($repoInfo.Owner)/$($repoInfo.Repo)/releases/$($release.id)" -Method Delete -Headers $headers | Out-Null
  }
  if ($tagPushed) {
    Write-Host "Deleting remote tag $newTag..."
    Invoke-Git -BestEffort @('push', 'origin', ":refs/tags/$newTag")
  }
  Write-Host "Deleting local tag $newTag..."
  Invoke-Git -BestEffort @('tag', '-d', $newTag)
  throw
}
