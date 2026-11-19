<#
.SYNOPSIS
  mdify'ı tüm platformlar için derler, dist/<sürüm>/ altına arşivler ve GitHub release oluşturur.

.EXAMPLE
  .\scripts\release.ps1 -Version 0.2.0            # derle + tag + push + GitHub release
  .\scripts\release.ps1 -Version 0.2.0 -NoPublish # sadece derle ve arşivle

  Gerekenler: Go, git, gh (gh auth login yapılmış), Windows 10+ (tar.exe).
  Release notu: dist\v<sürüm>\RELEASE_NOTES.md varsa o kullanılır.
#>
param(
    [Parameter(Mandatory)][string]$Version,
    [switch]$NoPublish,
    [string]$Repo = "NetYazilim/mdify"
)

$ErrorActionPreference = "Stop"
$Version = $Version.TrimStart("v")
$tag = "v$Version"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Run([string]$exe, [string[]]$argv) {
    & $exe @argv
    if ($LASTEXITCODE -ne 0) { throw "$exe $($argv -join ' ') başarısız (exit $LASTEXITCODE)" }
}

# --- ön kontroller
if (-not $NoPublish) {
    if (git status --porcelain --untracked-files=no) { throw "Commit edilmemiş değişiklikler var; önce commit edin." }
    if (git tag --list $tag) { throw "$tag tag'i zaten var." }
    Run gh @("auth", "status")
}

Write-Host "==> test"
Run go @("test", "./...")

# --- derleme
$dist = Join-Path $root "dist\$tag"
if (Test-Path $dist) {
    Get-ChildItem $dist -Exclude "RELEASE_NOTES.md" | Remove-Item -Recurse -Force
} else {
    New-Item -ItemType Directory -Path $dist | Out-Null
}

$targets = @(
    @{ os = "windows"; arch = "amd64" }, @{ os = "windows"; arch = "arm64" },
    @{ os = "linux";   arch = "amd64" }, @{ os = "linux";   arch = "arm64" },
    @{ os = "darwin";  arch = "amd64" }, @{ os = "darwin";  arch = "arm64" }
)

$oldEnv = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED }
try {
    foreach ($t in $targets) {
        $name = "mdify_${Version}_$($t.os)_$($t.arch)"
        $work = Join-Path $env:TEMP $name
        if (Test-Path $work) { Remove-Item $work -Recurse -Force }
        New-Item -ItemType Directory -Path $work | Out-Null

        $bin = if ($t.os -eq "windows") { "mdify.exe" } else { "mdify" }
        Write-Host "==> $($t.os)/$($t.arch)"
        $env:GOOS = $t.os; $env:GOARCH = $t.arch; $env:CGO_ENABLED = "0"
        Run go @("build", "-trimpath", "-ldflags", "-s -w -X main.version=$Version", "-o", (Join-Path $work $bin), ".")
        Copy-Item README.md $work

        if ($t.os -eq "windows") {
            Compress-Archive -Path "$work\*" -DestinationPath (Join-Path $dist "$name.zip") -Force
        } else {
            Run tar @("-czf", (Join-Path $dist "$name.tar.gz"), "-C", $work, $bin, "README.md")
        }
        Remove-Item $work -Recurse -Force
    }
} finally {
    $env:GOOS = $oldEnv.GOOS; $env:GOARCH = $oldEnv.GOARCH; $env:CGO_ENABLED = $oldEnv.CGO_ENABLED
}

# --- checksums (sha256sum biçimi, LF satır sonu)
$archives = Get-ChildItem $dist -File | Where-Object { $_.Name -match '\.(zip|tar\.gz)$' } | Sort-Object Name
$lines = $archives | ForEach-Object { "{0}  {1}" -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower(), $_.Name }
[IO.File]::WriteAllText((Join-Path $dist "checksums.txt"), (($lines -join "`n") + "`n"))
Write-Host "==> arşivler: $dist"

if ($NoPublish) { return }

# --- tag + release
Write-Host "==> tag $tag"
Run git @("tag", "-a", $tag, "-m", "mdify $tag")
Run git @("push", "origin", "HEAD")
Run git @("push", "origin", $tag)

$notes = Join-Path $dist "RELEASE_NOTES.md"
$assets = @($archives.FullName) + (Join-Path $dist "checksums.txt")
$ghArgs = @("release", "create", $tag, "--repo", $Repo, "--title", "mdify $tag", "--verify-tag")
if (Test-Path $notes) { $ghArgs += @("--notes-file", $notes) } else { $ghArgs += "--generate-notes" }
Run gh ($ghArgs + $assets)
Write-Host "==> yayınlandı: https://github.com/$Repo/releases/tag/$tag"
