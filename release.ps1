param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v\d+\.\d+\.\d+$')]
    [string]$Version,

    [string]$Repo = "hyperion13th144m/phantom-manager"
)

$ErrorActionPreference = "Stop"

$ProjectDir = $PSScriptRoot
$RootDir = Split-Path -Parent $ProjectDir
$DistDir = Join-Path $RootDir "dist"
$PublishDir = Join-Path $DistDir "phantom-manager-$Version-win-x64"
$ZipPath = Join-Path $DistDir "phantom-manager-$Version-win-x64.zip"

Push-Location $ProjectDir
try {
    $status = git status --short
    if ($status) {
        throw "Working tree is not clean. Commit changes before releasing."
    }

    if (git tag --list $Version) {
        throw "Tag $Version already exists."
    }

    New-Item -ItemType Directory -Force -Path $DistDir | Out-Null
    if (Test-Path $PublishDir) {
        Remove-Item -LiteralPath $PublishDir -Recurse -Force
    }
    if (Test-Path $ZipPath) {
        Remove-Item -LiteralPath $ZipPath -Force
    }

    dotnet publish `
        -c Release `
        -r win-x64 `
        --self-contained true `
        -p:PublishSingleFile=true `
        -p:AssemblyName=phantom-manager `
        -p:EnableCompressionInSingleFile=true `
        -o $PublishDir

    Compress-Archive -Path (Join-Path $PublishDir "*") -DestinationPath $ZipPath -Force

    git tag $Version
    git push origin $Version

    gh release create $Version $ZipPath `
        --repo $Repo `
        --title $Version `
        --notes "Release $Version of phantom-manager."

    Write-Host "Release uploaded: https://github.com/$Repo/releases/tag/$Version"
    Write-Host "Asset: $ZipPath"
}
finally {
    Pop-Location
}
