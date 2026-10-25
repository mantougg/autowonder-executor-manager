# install.ps1：awem 一键安装（PowerShell；T11-F2）。
# 用法：
#   irm <repo>/raw/main/scripts/install.ps1 | iex                    # 最新版
#   & .\install.ps1 -Version v0.1.0                                   # 指定版本
param(
  [string]$Version = "latest",
  [string]$Repo = "mantougg/awem"
)

$ErrorActionPreference = "Stop"

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { "amd64" }
  "ARM64" { "arm64" }
  default { throw "不支持的架构：$($env:PROCESSOR_ARCHITECTURE)（支持 amd64/arm64）" }
}
$Asset = "awem-windows-$Arch.exe"

if ($Version -eq "latest") {
  $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
  $Version = $rel.tag_name
}
$BaseUrl = "https://github.com/$Repo/releases/download/$Version"

$BinDir = Join-Path $env:LOCALAPPDATA "awem"
$Tmp = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP ("awem-" + [guid]::NewGuid().ToString("N").Substring(0, 8)))
try {
  Write-Host ">> 下载 $BaseUrl/$Asset"
  Invoke-WebRequest "$BaseUrl/$Asset" -OutFile (Join-Path $Tmp $Asset)
  Write-Host ">> 下载 $BaseUrl/checksums.txt"
  Invoke-WebRequest "$BaseUrl/checksums.txt" -OutFile (Join-Path $Tmp "checksums.txt")

  # 校验 sha256
  $want = (Select-String -Path (Join-Path $Tmp "checksums.txt") -Pattern $Asset |
    ForEach-Object { ($_.Line -split '\s+')[0] }) | Select-Object -First 1
  if (-not $want) { throw "checksums.txt 中无 $Asset 条目，拒绝安装" }
  $got = (Get-FileHash (Join-Path $Tmp $Asset) -Algorithm SHA256).Hash.ToLower()
  if ($got -ne $want.ToLower()) {
    throw "sha256 不匹配：期望 $want，实际 $got —— 拒绝安装"
  }

  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  Move-Item (Join-Path $Tmp $Asset) (Join-Path $BinDir "awem.exe") -Force
  Write-Host ">> 已安装 $(Join-Path $BinDir 'awem.exe')（$Version）"
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if ($userPath -notlike "*$BinDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$BinDir", "User")
    Write-Host "提示：已把 $BinDir 加入用户 PATH（新开终端生效）"
  }
  & (Join-Path $BinDir "awem.exe") version
} finally {
  Remove-Item $Tmp -Recurse -Force -ErrorAction SilentlyContinue
}
