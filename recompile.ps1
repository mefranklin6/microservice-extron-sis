# Windows dev script that removes any old container and rebuilds a new one with your changes
# Run when you update the code and need to deploy a new test container
# Use only for testing and development

$ErrorActionPreference = 'Stop'

# Ensure Docker CLI is available
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Error "Docker CLI not found in PATH."
    exit 1
}

# Run from the repo root (script directory)
Set-Location -Path $PSScriptRoot

$image = "microservice-extron-sis"
$portMapping = "80:80"

# Find container(s) (running or stopped) for this image
$containers = docker ps -aqf "ancestor=$image"
if ($containers) {
    Write-Host "Stopping and removing existing containers for image '$image'..."
    $containers -split "\r?\n" | Where-Object { $_ } | ForEach-Object {
        try { docker stop $_ | Out-Null } catch {}
        try { docker rm $_   | Out-Null } catch {}
    }
}

# Rebuild and run
Write-Host "Building image '$image'..."
docker build -t $image .

Write-Host "Starting new container..."
docker run -d -p $portMapping $image

Write-Host "Done."