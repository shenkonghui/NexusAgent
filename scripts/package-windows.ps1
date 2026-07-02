# Windows 桌面应用打包脚本
#
# 将 Go 后端 + Pake 客户端组合为可分发目录:
#   NexusAgent/
#     nexusagent.exe
#     NexusAgent-Client.exe
#     config.yaml
#     web/
#     launch.ps1
#
# 用法: pwsh ./scripts/package-windows.ps1 -OutDir dist -BinName nexusagent-windows-amd64.exe

param(
    [string]$OutDir = "dist",
    [string]$BinName = "nexusagent-windows-amd64.exe",
    [string]$PakeDir = "dist"
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$AppDir = Join-Path $OutDir "NexusAgent"
$ClientName = "NexusAgent-Client"

$BackendBin = Join-Path $OutDir $BinName
if (-not (Test-Path $BackendBin)) {
    Write-Error "未找到后端 binary: $BackendBin"
}

$ClientExe = Get-ChildItem -Path $PakeDir -Filter "$ClientName*.exe" -File -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $ClientExe) {
    Write-Error "未找到 Pake 客户端: $PakeDir\$ClientName*.exe，请先运行 build-pake.sh"
}

Write-Host "==> 创建 Windows 桌面应用目录..."
if (Test-Path $AppDir) { Remove-Item -Recurse -Force $AppDir }
New-Item -ItemType Directory -Path $AppDir | Out-Null

Copy-Item $BackendBin (Join-Path $AppDir "nexusagent.exe")
Copy-Item $ClientExe.FullName (Join-Path $AppDir $ClientExe.Name)

$ConfigSrc = Join-Path $ScriptDir "config.yaml"
if (Test-Path $ConfigSrc) {
    Copy-Item $ConfigSrc (Join-Path $AppDir "config.yaml")
}

$WebSrc = Join-Path $ScriptDir "web/dist"
if (Test-Path $WebSrc) {
    Copy-Item -Recurse $WebSrc (Join-Path $AppDir "web")
} else {
    Write-Warning "未找到前端构建产物 web/dist"
}

@'
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Binary = Join-Path $Root "nexusagent.exe"
$Client = Get-ChildItem -Path $Root -Filter "NexusAgent-Client*.exe" -File | Select-Object -First 1
$DataDir = Join-Path $env:LOCALAPPDATA "NexusAgent"
$LogFile = Join-Path $DataDir "launcher.log"
$Port = 8080

New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
Start-Transcript -Path $LogFile -Append | Out-Null

Write-Host "NexusAgent 启动于 $(Get-Date)"

$conn = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue
if ($conn) {
    Stop-Process -Id $conn.OwningProcess -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
}

$env:CONFIG_PATH = Join-Path $Root "config.yaml"
$env:SERVER_MODE = "release"
$env:WEB_DIST = Join-Path $Root "web"

$backend = Start-Process -FilePath $Binary -ArgumentList @("--data-dir", $DataDir) -PassThru -WindowStyle Hidden

$ready = $false
for ($i = 0; $i -lt 60; $i++) {
    try {
        Invoke-WebRequest -Uri "http://127.0.0.1:$Port/health" -UseBasicParsing -TimeoutSec 2 | Out-Null
        $ready = $true
        break
    } catch {
        if ($backend.HasExited) { throw "后端启动失败" }
        Start-Sleep -Milliseconds 500
    }
}

if (-not $ready) {
    Stop-Process -Id $backend.Id -Force -ErrorAction SilentlyContinue
    throw "后端启动超时"
}

if ($Client) {
    $clientProc = Start-Process -FilePath $Client.FullName -PassThru
    Wait-Process -Id $clientProc.Id
} else {
    Start-Process "http://127.0.0.1:$Port"
    Wait-Process -Id $backend.Id
}

Stop-Process -Id $backend.Id -Force -ErrorAction SilentlyContinue
Stop-Transcript | Out-Null
'@ | Set-Content -Path (Join-Path $AppDir "launch.ps1") -Encoding UTF8

@'
@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0launch.ps1"
'@ | Set-Content -Path (Join-Path $AppDir "launch.bat") -Encoding ASCII

Write-Host ""
Write-Host "✅ Windows 桌面应用已创建: $AppDir"
Write-Host "   运行: $AppDir\launch.bat"
