#requires -Version 5.1
<#
.SYNOPSIS
    测量 Hephaestus Windows 桌面客户端的启动耗时。

.DESCRIPTION
    记录从 Start-Process 到"主窗口出现"以及"WebView2 子窗口出现"的时间，
    同时采样进程 CPU 时间、读取字节数、Hephaestus / msedgewebview2 进程数量。
    这些数据用于判断耗时主要花在：
      - 应用自身初始化（CPU 时间接近窗口耗时）
      - 安全软件扫描 / 冷磁盘读取（CPU 时间很小但读取量很大）
      - WebView2 冷启动（窗口出现后到 WebView2 子窗口出现的间隔）

    可选 -SecondLaunchDelayMs 用来复现"再次双击"：脚本会在指定延迟后再启动一个
    实例，并记录它被阻塞了多久（Windows 单实例通知是同步的）。

.PARAMETER ExePath
    Hephaestus.exe 路径。省略时依次尝试常见安装位置和脚本所在目录。

.PARAMETER Runs
    连续测试次数，默认 3。建议重启后先跑一次冷启动，再跑热启动做对比。

.PARAMETER TimeoutSeconds
    单次等待窗口的超时时间，默认 180 秒。

.PARAMETER CsvPath
    结果 CSV 路径。省略时写入当前目录。

.PARAMETER TimelinePath
    可选。按秒采样的时间线 CSV 前缀，每次运行会生成 <前缀>-run<N>.csv。

.PARAMETER SecondLaunchDelayMs
    大于 0 时，在启动后该毫秒数再启动一个实例，用于复现"多次双击"。

.PARAMETER KeepRunning
    测量结束后不关闭应用。

.PARAMETER KillExisting
    开始前强制结束已存在的 Hephaestus 进程。

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\measure-desktop-startup.ps1 -Runs 3 -KillExisting

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\measure-desktop-startup.ps1 `
        -ExePath "C:\Users\me\Desktop\Hephaestus\Hephaestus.exe" -SecondLaunchDelayMs 300
#>
[CmdletBinding()]
param(
    [string]$ExePath = "",
    [int]$Runs = 3,
    [int]$TimeoutSeconds = 180,
    [string]$CsvPath = "",
    [string]$TimelinePath = "",
    [int]$SecondLaunchDelayMs = 0,
    [switch]$KeepRunning,
    [switch]$KillExisting
)

$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Win32 helpers: 找到进程的主窗口、枚举子窗口、读取窗口标题
# ---------------------------------------------------------------------------
if (-not ("HephaestusStartupProbe.Win32" -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

namespace HephaestusStartupProbe
{
    public static class Win32
    {
        private const uint GW_OWNER = 4;
        private delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);

        [DllImport("user32.dll")]
        private static extern bool EnumWindows(EnumWindowsProc callback, IntPtr lParam);

        [DllImport("user32.dll")]
        private static extern bool EnumChildWindows(IntPtr parent, EnumWindowsProc callback, IntPtr lParam);

        [DllImport("user32.dll")]
        private static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint processId);

        [DllImport("user32.dll")]
        private static extern bool IsWindowVisible(IntPtr hWnd);

        [DllImport("user32.dll")]
        private static extern IntPtr GetWindow(IntPtr hWnd, uint command);

        [DllImport("user32.dll")]
        private static extern int GetWindowTextLength(IntPtr hWnd);

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetWindowText(IntPtr hWnd, StringBuilder text, int maxCount);

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetClassName(IntPtr hWnd, StringBuilder name, int maxCount);

        // 找到属于该进程的顶层可见窗口（Skip 掉 Wails 的隐藏消息窗口和消息-only 窗口）。
        public static IntPtr FindMainWindow(int processId)
        {
            IntPtr result = IntPtr.Zero;
            EnumWindows(delegate(IntPtr hWnd, IntPtr lParam)
            {
                uint ownerPid;
                GetWindowThreadProcessId(hWnd, out ownerPid);
                if (ownerPid != (uint)processId) return true;
                if (!IsWindowVisible(hWnd)) return true;

                // GW_OWNER = 4：排除有 owner 的辅助窗口
                if (GetWindow(hWnd, GW_OWNER) != IntPtr.Zero) return true;

                // 无标题的窗口基本是内部辅助窗口
                if (GetWindowTextLength(hWnd) == 0) return true;

                result = hWnd;
                return false;
            }, IntPtr.Zero);
            return result;
        }

        public static string GetTitle(IntPtr hWnd)
        {
            StringBuilder builder = new StringBuilder(512);
            GetWindowText(hWnd, builder, builder.Capacity);
            return builder.ToString();
        }

        public static string[] ChildClasses(IntPtr parent)
        {
            List<string> classes = new List<string>();
            EnumChildWindows(parent, delegate(IntPtr hWnd, IntPtr lParam)
            {
                StringBuilder builder = new StringBuilder(256);
                GetClassName(hWnd, builder, builder.Capacity);
                classes.Add(builder.ToString());
                return true;
            }, IntPtr.Zero);
            return classes.ToArray();
        }
    }
}
'@
}

# ---------------------------------------------------------------------------
# 基础工具
# ---------------------------------------------------------------------------
function Resolve-HephaestusExe {
    param([string]$Path)

    if ($Path) {
        if (-not (Test-Path -LiteralPath $Path)) { throw "找不到可执行文件: $Path" }
        return (Resolve-Path -LiteralPath $Path).Path
    }

    $candidates = New-Object System.Collections.Generic.List[string]
    $candidates.Add((Join-Path $PSScriptRoot "Hephaestus.exe"))
    if ($env:LOCALAPPDATA) { $candidates.Add((Join-Path $env:LOCALAPPDATA "Programs\Hephaestus\Hephaestus.exe")) }
    if ($env:ProgramFiles) { $candidates.Add((Join-Path $env:ProgramFiles "Hephaestus\Hephaestus.exe")) }
    if (${env:ProgramFiles(x86)}) { $candidates.Add((Join-Path ${env:ProgramFiles(x86)} "Hephaestus\Hephaestus.exe")) }

    foreach ($candidate in $candidates) {
        if ($candidate -and (Test-Path -LiteralPath $candidate)) {
            return (Resolve-Path -LiteralPath $candidate).Path
        }
    }

    throw "没有自动找到 Hephaestus.exe，请用 -ExePath 指定路径。"
}

function Get-ProcessSnapshot {
    param([int]$ProcessId)

    $process = $null
    try {
        $process = Get-Process -Id $ProcessId -ErrorAction Stop
    } catch {
        return $null
    }

    $readMB = $null
    $otherMB = $null
    try {
        $wmi = Get-CimInstance Win32_Process -Filter "ProcessId=$ProcessId" -ErrorAction Stop
        if ($null -ne $wmi -and $null -ne $wmi.ReadTransferCount) {
            $readMB = [math]::Round(([double]$wmi.ReadTransferCount) / 1MB, 1)
        }
        if ($null -ne $wmi -and $null -ne $wmi.OtherTransferCount) {
            $otherMB = [math]::Round(([double]$wmi.OtherTransferCount) / 1MB, 1)
        }
    } catch {
        $readMB = $null
    }

    return [pscustomobject]@{
        CpuSeconds   = [math]::Round($process.TotalProcessorTime.TotalSeconds, 2)
        WorkingSetMB = [math]::Round($process.WorkingSet64 / 1MB, 1)
        ReadMB       = $readMB
        OtherMB      = $otherMB
    }
}

function Get-ProcessCount {
    param([string]$Name)

    try {
        return @(Get-Process -Name $Name -ErrorAction Stop).Count
    } catch {
        return 0
    }
}

function Get-ProcessCpuSeconds {
    param([string]$Name)

    try {
        $process = @(Get-Process -Name $Name -ErrorAction Stop) | Select-Object -First 1
        if ($process) {
            return $process.TotalProcessorTime.TotalSeconds
        }
    } catch {
        return $null
    }
    return $null
}

# ---------------------------------------------------------------------------
# 环境信息
# ---------------------------------------------------------------------------
$exe = Resolve-HephaestusExe $ExePath
$procName = [System.IO.Path]::GetFileNameWithoutExtension($exe)
$exeInfo = Get-Item -LiteralPath $exe
$osInfo = Get-CimInstance Win32_OperatingSystem
$bootMinutes = ((Get-Date) - $osInfo.LastBootUpTime).TotalMinutes

Write-Host ""
Write-Host "Hephaestus 启动耗时测量" -ForegroundColor Cyan
Write-Host "----------------------------------------"
Write-Host ("可执行文件 : {0}" -f $exe)
Write-Host ("文件大小   : {0:N0} MB" -f ($exeInfo.Length / 1MB))
Write-Host ("本次开机   : {0:yyyy-MM-dd HH:mm:ss}（已开机 {1:N1} 分钟）" -f $osInfo.LastBootUpTime, $bootMinutes)
Write-Host ("测试次数   : {0}" -f $Runs)

try {
    $signature = Get-AuthenticodeSignature -FilePath $exe
    Write-Host ("数字签名   : {0}" -f $signature.Status)
} catch {
    Write-Host "数字签名   : 读取失败"
}

$zoneStream = $null
try {
    $zoneStream = Get-Item -LiteralPath $exe -Stream Zone.Identifier -ErrorAction SilentlyContinue
} catch {
    $zoneStream = $null
}
if ($zoneStream) {
    Write-Host "注意       : 该文件带 Mark-of-the-Web（浏览器下载标记），首次运行可能触发 SmartScreen / Defender 云检查。" -ForegroundColor Yellow
}

try {
    $defender = Get-MpComputerStatus -ErrorAction Stop
    Write-Host ("Defender   : 实时保护={0} 防病毒={1}" -f $defender.RealTimeProtectionEnabled, $defender.AntivirusEnabled)
} catch {
    Write-Host "Defender   : 无法读取状态（可能未安装或权限不足）"
}

try {
    $avProducts = @(Get-CimInstance -Namespace root/SecurityCenter2 -ClassName AntiVirusProduct -ErrorAction Stop)
    foreach ($av in $avProducts) {
        Write-Host ("安全软件   : {0}（state={1}）" -f $av.displayName, $av.productState)
    }
} catch {
    # 非必需信息，读取失败忽略
}

try {
    $disks = @(Get-PhysicalDisk -ErrorAction Stop | Select-Object -First 3)
    foreach ($disk in $disks) {
        Write-Host ("磁盘       : {0}（{1}, {2}）" -f $disk.FriendlyName, $disk.MediaType, $disk.BusType)
    }
} catch {
    # 非必需信息
}

$configPath = Join-Path $env:USERPROFILE ".config\hephaestus\config-desktop.yaml"
if (Test-Path -LiteralPath $configPath) {
    $inDatabase = $false
    $databaseLines = @()
    foreach ($line in Get-Content -LiteralPath $configPath) {
        if ($line -match '^database:\s*$') {
            $inDatabase = $true
            continue
        }
        if ($inDatabase) {
            if ($line -match '^\S') { break }
            $trimmed = $line.Trim()
            if (-not $trimmed) { continue }
            if ($trimmed -match '^password:') { $trimmed = 'password: ***' }
            $databaseLines += $trimmed
        }
    }
    if ($databaseLines.Count -gt 0) {
        Write-Host ("数据库配置 : {0}" -f ($databaseLines -join ' '))
    } else {
        Write-Host "数据库配置 : 未显式配置（桌面端默认 SQLite）"
    }
} else {
    Write-Host ("数据库配置 : 未找到 {0}（默认 SQLite）" -f $configPath)
}

$dbPath = Join-Path $env:USERPROFILE ".config\hephaestus\data\hephaestus.db"
if (Test-Path -LiteralPath $dbPath) {
    Write-Host ("SQLite     : {0:N0} MB" -f ((Get-Item -LiteralPath $dbPath).Length / 1MB))
    $walPath = "$dbPath-wal"
    if (Test-Path -LiteralPath $walPath) {
        Write-Host ("SQLite WAL : {0:N0} MB" -f ((Get-Item -LiteralPath $walPath).Length / 1MB))
    }
}

$pocsDir = Join-Path $env:USERPROFILE ".config\hephaestus\config\pocs"
$pocCount = 0
$pocMB = 0
try {
    $pocFiles = @(Get-ChildItem -LiteralPath $pocsDir -Recurse -File -Include *.yaml, *.yml -ErrorAction SilentlyContinue)
    $pocCount = $pocFiles.Count
    if ($pocCount -gt 0) {
        $pocMB = [math]::Round((($pocFiles | Measure-Object -Property Length -Sum).Sum) / 1MB, 1)
    }
} catch {
    $pocCount = 0
}
Write-Host ("PoC 模板   : {0} 个 / {1} MB（{2}）" -f $pocCount, $pocMB, $pocsDir)

Write-Host "----------------------------------------"

# ---------------------------------------------------------------------------
# 开始测量
# ---------------------------------------------------------------------------
$results = New-Object System.Collections.Generic.List[object]

for ($run = 1; $run -le $Runs; $run++) {
    $existing = @(Get-Process -Name $procName -ErrorAction SilentlyContinue)
    if ($existing.Count -gt 0) {
        if ($KillExisting) {
            Write-Host ("关闭已有的 {0} 个 {1} 进程..." -f $existing.Count, $procName) -ForegroundColor Yellow
            $existing | Stop-Process -Force -ErrorAction SilentlyContinue
            Start-Sleep -Milliseconds 1200
        } else {
            throw ("检测到正在运行的 {0} 进程（PID: {1}）。请先手动退出，或加 -KillExisting 让脚本自动关闭。" -f $procName, (($existing | ForEach-Object { $_.Id }) -join ", "))
        }
    }

    Write-Host ""
    Write-Host ("第 {0}/{1} 次启动..." -f $run, $Runs) -ForegroundColor Cyan

    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    $startedAt = Get-Date
    $launchArgs = @{
        FilePath         = $exe
        WorkingDirectory = (Split-Path -Parent $exe)
        PassThru         = $true
    }
    $defenderCpuBefore = Get-ProcessCpuSeconds -Name "MsMpEng"
    $createStopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    $process = Start-Process @launchArgs
    $createStopwatch.Stop()
    $createProcessSeconds = [math]::Round($createStopwatch.Elapsed.TotalSeconds, 2)
    $processStartDelaySeconds = $null
    try {
        $processStartDelaySeconds = [math]::Round(($process.StartTime - $startedAt).TotalSeconds, 2)
    } catch {
        $processStartDelaySeconds = $null
    }

    $windowHandle = [IntPtr]::Zero
    $windowTitle = ""
    $windowSeconds = $null
    $webviewSeconds = $null
    $secondProcess = $null
    $secondStartSeconds = $null
    $secondExitSeconds = $null
    $timedOut = $false
    $lastSampleSeconds = -10.0
    $timeline = New-Object System.Collections.Generic.List[object]

    while ($true) {
        if ($stopwatch.Elapsed.TotalSeconds -ge $TimeoutSeconds) {
            $timedOut = $true
            break
        }
        if ($process.HasExited) {
            break
        }

        if ($windowHandle -eq [IntPtr]::Zero) {
            try {
                $process.Refresh()
                $handle = [IntPtr]$process.MainWindowHandle
            } catch {
                break
            }
            if ($handle -eq [IntPtr]::Zero) {
                $handle = [HephaestusStartupProbe.Win32]::FindMainWindow($process.Id)
            }
            if ($handle -ne [IntPtr]::Zero) {
                $windowHandle = $handle
                $windowSeconds = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
                $windowTitle = [HephaestusStartupProbe.Win32]::GetTitle($handle)
                if ($windowTitle -match "失败|错误|Error") {
                    break
                }
            }
        } elseif ($null -eq $webviewSeconds) {
            $classes = [HephaestusStartupProbe.Win32]::ChildClasses($windowHandle)
            foreach ($className in $classes) {
                if ($className -like "*Chrome_WidgetWin*" -or $className -like "*WebView2*") {
                    $webviewSeconds = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
                    break
                }
            }
        }

        if ($SecondLaunchDelayMs -gt 0 -and $null -eq $secondProcess -and $stopwatch.Elapsed.TotalMilliseconds -ge $SecondLaunchDelayMs) {
            $secondProcess = Start-Process @launchArgs
            $secondStartSeconds = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
        }
        if ($null -ne $secondProcess -and $null -eq $secondExitSeconds -and $secondProcess.HasExited) {
            $secondExitSeconds = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
        }

        if (($stopwatch.Elapsed.TotalSeconds - $lastSampleSeconds) -ge 1.0) {
            $lastSampleSeconds = $stopwatch.Elapsed.TotalSeconds
            $snapshot = Get-ProcessSnapshot -ProcessId $process.Id
            $cpuSeconds = $null
            $readMB = $null
            $otherMB = $null
            $workingSetMB = $null
            if ($snapshot) {
                $cpuSeconds = $snapshot.CpuSeconds
                $readMB = $snapshot.ReadMB
                $otherMB = $snapshot.OtherMB
                $workingSetMB = $snapshot.WorkingSetMB
            }
            $timeline.Add([pscustomobject]@{
                ElapsedSec         = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
                HephaestusCount    = (Get-ProcessCount -Name $procName)
                WebView2Count      = (Get-ProcessCount -Name "msedgewebview2")
                CpuSeconds         = $cpuSeconds
                ReadMB             = $readMB
                OtherMB            = $otherMB
                WorkingSetMB       = $workingSetMB
                DefenderCpuSeconds = (Get-ProcessCpuSeconds -Name "MsMpEng")
                WindowShown        = ($windowHandle -ne [IntPtr]::Zero)
                WebViewReady       = ($null -ne $webviewSeconds)
            })
        }

        $windowDone = ($windowHandle -ne [IntPtr]::Zero) -and ($null -ne $webviewSeconds)
        $secondDone = ($null -eq $secondProcess) -or ($null -ne $secondExitSeconds)
        if ($windowDone -and $secondDone) {
            break
        }

        Start-Sleep -Milliseconds 100
    }

    $wallSeconds = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
    $finalSnapshot = Get-ProcessSnapshot -ProcessId $process.Id
    $finalCpu = $null
    $finalRead = $null
    $finalWorkingSet = $null
    if ($finalSnapshot) {
        $finalCpu = $finalSnapshot.CpuSeconds
        $finalRead = $finalSnapshot.ReadMB
        $finalWorkingSet = $finalSnapshot.WorkingSetMB
    }

    # 第二个实例如果仍在运行，给它一点时间退出（它可能阻塞在单实例 SendMessage 上）
    if ($null -ne $secondProcess -and $null -eq $secondExitSeconds) {
        if ($secondProcess.WaitForExit(15000)) {
            $secondExitSeconds = [math]::Round($stopwatch.Elapsed.TotalSeconds, 2)
        }
    }

    $secondBlockedSeconds = $null
    if ($null -ne $secondStartSeconds -and $null -ne $secondExitSeconds) {
        $secondBlockedSeconds = [math]::Round($secondExitSeconds - $secondStartSeconds, 2)
    }

    $defenderCpuAfter = Get-ProcessCpuSeconds -Name "MsMpEng"
    $defenderCpuDelta = $null
    if ($null -ne $defenderCpuBefore -and $null -ne $defenderCpuAfter) {
        $defenderCpuDelta = [math]::Round($defenderCpuAfter - $defenderCpuBefore, 2)
    }

    $windowAfterCreateSeconds = $null
    if ($null -ne $windowSeconds) {
        $windowAfterCreateSeconds = [math]::Round($windowSeconds - $createProcessSeconds, 2)
    }

    if ($null -eq $windowSeconds) {
        if ($process.HasExited) {
            Write-Host "  进程在窗口出现前退出，请检查日志或错误弹窗" -ForegroundColor Red
        } else {
            Write-Host ("  超时：{0} 秒内没有检测到主窗口" -f $TimeoutSeconds) -ForegroundColor Red
        }
    } else {
        Write-Host ("  进程创建      : {0} 秒" -f $createProcessSeconds) -ForegroundColor Green
        if ($createProcessSeconds -ge 2) {
            Write-Host "      ^ 创建阶段本身就超过 2 秒：杀软/加载器在 CreateProcess 阶段阻塞，与应用代码无关。" -ForegroundColor Yellow
        }
        Write-Host ("  创建后到窗口  : {0} 秒" -f ([math]::Round($windowSeconds - $createProcessSeconds, 2)))
        if ($null -ne $webviewSeconds) {
            Write-Host ("  WebView2 就绪 : {0} 秒" -f $webviewSeconds)
        } else {
            Write-Host "  WebView2 就绪 : 未检测到（窗口可能只是空白/错误弹窗）" -ForegroundColor Yellow
        }
        if ($null -ne $secondBlockedSeconds) {
            Write-Host ("  第二次双击被阻塞: {0} 秒" -f $secondBlockedSeconds) -ForegroundColor Yellow
        }
        if ($null -eq $defenderCpuDelta) {
            Write-Host "  Defender CPU  : 无法读取（用管理员权限重跑可拿到这个值）" -ForegroundColor Yellow
        } elseif ($defenderCpuDelta -ge 1) {
            Write-Host ("  Defender CPU  : {0} 秒" -f $defenderCpuDelta) -ForegroundColor Yellow
        }
        if ($windowTitle -and $windowTitle -notlike "*Hephaestus*") {
            Write-Host ("  警告: 主窗口标题是 '{0}'，可能是启动失败弹窗" -f $windowTitle) -ForegroundColor Yellow
        }
    }

    $results.Add([pscustomobject]@{
        Run                    = $run
        StartedAt              = $startedAt.ToString("yyyy-MM-dd HH:mm:ss.fff")
        BootMinutes            = [math]::Round($bootMinutes, 1)
        CreateProcessSec       = $createProcessSeconds
        ProcessStartDelaySec   = $processStartDelaySeconds
        TimeToWindowSec        = $windowSeconds
        WindowAfterCreateSec   = $windowAfterCreateSeconds
        TimeToWebViewSec       = $webviewSeconds
        WindowTitle            = $windowTitle
        CpuSecondsAtEnd        = $finalCpu
        ReadMBAtEnd            = $finalRead
        WorkingSetMBAtEnd      = $finalWorkingSet
        SecondLaunchBlockedSec = $secondBlockedSeconds
        DefenderCpuDeltaSec    = $defenderCpuDelta
        TimedOut               = ($null -eq $windowSeconds)
        WallSeconds            = $wallSeconds
    })

    if ($TimelinePath) {
        $timelineFile = "{0}-run{1}.csv" -f $TimelinePath, $run
        $timeline | Export-Csv -Path $timelineFile -NoTypeInformation -Encoding UTF8
        Write-Host ("  时间线已写入: {0}" -f $timelineFile)
    }

    if (-not $KeepRunning) {
        try { $null = $process.CloseMainWindow() } catch { }
        Start-Sleep -Milliseconds 800
        try { $null = $process.WaitForExit(8000) } catch { }
        if (-not $process.HasExited) {
            try { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue } catch { }
        }
        if ($null -ne $secondProcess -and -not $secondProcess.HasExited) {
            try { Stop-Process -Id $secondProcess.Id -Force -ErrorAction SilentlyContinue } catch { }
        }
    }
}

Write-Host ""
Write-Host "结果汇总" -ForegroundColor Cyan
$results | Format-Table -AutoSize

if (-not $CsvPath) {
    $CsvPath = Join-Path (Get-Location) ("hephaestus-startup-{0}-{1}.csv" -f $env:COMPUTERNAME, (Get-Date -Format "yyyyMMdd-HHmmss"))
}
$results | Export-Csv -Path $CsvPath -NoTypeInformation -Encoding UTF8
Write-Host ("CSV: {0}" -f $CsvPath)

Write-Host ""
Write-Host "怎么看这些数字：" -ForegroundColor Cyan
Write-Host "  CreateProcessSec 很大（>=2s）"
Write-Host "      -> Start-Process 本身就被阻塞了：杀软/加载器在进程创建阶段卡住，应用代码还没开始跑。"
Write-Host "  CreateProcessSec 很小，但 WindowAfterCreateSec 大且 CpuSecondsAtEnd 很小"
Write-Host "      -> 进程已经起来，但在等外部资源（磁盘冷读、数据库/网络、文件锁）。"
Write-Host "  CpuSecondsAtEnd 接近 WindowAfterCreateSec"
Write-Host "      -> 时间花在应用自身初始化（数据库迁移、任务恢复、模板缓存等）。"
Write-Host "  SecondLaunchBlockedSec 很大"
Write-Host "      -> 再次双击只是阻塞在 Windows 单实例通知上，不会让窗口更早出现。"
Write-Host "  DefenderCpuDeltaSec 和窗口耗时接近"
Write-Host "      -> 主要时间花在 Microsoft Defender 扫描未签名的大 exe 上。"
