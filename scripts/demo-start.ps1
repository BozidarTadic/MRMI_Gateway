param(
    [int]$UiPort = 5294,
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$gatewayProject = Join-Path $repoRoot "cmd\mrmi-gateway"
$demoProject = Join-Path $repoRoot "demo\blazor\MRMI.Demo.Blazor\MRMI.Demo.Blazor.csproj"
$rsConfig = Join-Path $repoRoot "configs\node.rs.demo.toml"
$ruConfig = Join-Path $repoRoot "configs\node.ru.demo.toml"
$logDir = Join-Path $repoRoot "logs\demo"

function Test-PortAvailable {
    param([int]$Port)

    $listener = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
    return $null -eq $listener
}

function Require-Port {
    param(
        [int]$Port,
        [string]$Name
    )

    if (-not (Test-PortAvailable -Port $Port)) {
        throw "$Name port $Port is already in use. Stop the existing process or choose another port."
    }
}

Require-Port -Port 8080 -Name "RS HTTP"
Require-Port -Port 8081 -Name "RU HTTP"
Require-Port -Port 7787 -Name "RS gRPC"
Require-Port -Port 7788 -Name "RU gRPC"
Require-Port -Port $UiPort -Name "Blazor UI"

if (-not $NoBuild) {
    Push-Location $repoRoot
    try {
        dotnet build $demoProject
        go build ./cmd/mrmi-gateway
    }
    finally {
        Pop-Location
    }
}

New-Item -ItemType Directory -Force -Path $logDir | Out-Null

$processes = @()
$pushed = $false

try {
    Push-Location $repoRoot
    $pushed = $true

    $processes += Start-Process -FilePath "go" `
        -ArgumentList @("run", "./cmd/mrmi-gateway", "-config", $rsConfig) `
        -WorkingDirectory $repoRoot `
        -RedirectStandardOutput (Join-Path $logDir "rs-node.out.log") `
        -RedirectStandardError (Join-Path $logDir "rs-node.err.log") `
        -WindowStyle Hidden `
        -PassThru

    $processes += Start-Process -FilePath "go" `
        -ArgumentList @("run", "./cmd/mrmi-gateway", "-config", $ruConfig) `
        -WorkingDirectory $repoRoot `
        -RedirectStandardOutput (Join-Path $logDir "ru-node.out.log") `
        -RedirectStandardError (Join-Path $logDir "ru-node.err.log") `
        -WindowStyle Hidden `
        -PassThru

    $processes += Start-Process -FilePath "dotnet" `
        -ArgumentList @("run", "--project", $demoProject, "--no-build", "--urls", "http://localhost:$UiPort") `
        -WorkingDirectory $repoRoot `
        -RedirectStandardOutput (Join-Path $logDir "blazor-ui.out.log") `
        -RedirectStandardError (Join-Path $logDir "blazor-ui.err.log") `
        -WindowStyle Hidden `
        -PassThru

    Write-Host ""
    Write-Host "MRMI demo started."
    Write-Host "  RS node:    http://localhost:8080"
    Write-Host "  RS UI:      http://localhost:8080/ui/"
    Write-Host "  RU node:    http://localhost:8081"
    Write-Host "  RU UI:      http://localhost:8081/ui/"
    Write-Host "  Blazor UI:  http://localhost:$UiPort"
    Write-Host "  Logs:       $logDir"
    Write-Host ""
    Write-Host "Press Ctrl+C in this window to stop all demo processes."

    while ($true) {
        Start-Sleep -Seconds 1
        foreach ($process in $processes) {
            if ($process.HasExited) {
                throw "Process $($process.Id) exited."
            }
        }
    }
}
finally {
    if ($pushed) {
        Pop-Location
    }

    foreach ($process in $processes) {
        if (-not $process.HasExited) {
            Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
        }
    }
}
