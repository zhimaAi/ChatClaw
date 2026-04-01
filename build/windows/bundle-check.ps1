param(
    [string]$ZipPath,
    [string]$Arch,
    [string]$ProjectRoot
)

$bundleDir = Split-Path $ZipPath -Parent

if (Test-Path $ZipPath) {
    Write-Host "Zip exists, skipping bundle."
} else {
    Write-Host "Bundling OpenClaw runtime..."
    Push-Location $bundleDir
    try {
        go run "$ProjectRoot\internal\tools\openclawbundle" -config "$ProjectRoot\build\runtime.yml" -os windows -arch $Arch
    } finally {
        Pop-Location
    }

    if (-not (Test-Path $ZipPath)) {
        Compress-Archive -Path "$bundleDir\windows-$Arch\*" -DestinationPath $ZipPath -Force
        Write-Host "Zipped runtime."
    } else {
        Write-Host "Zip exists, skipping compression."
    }
}
