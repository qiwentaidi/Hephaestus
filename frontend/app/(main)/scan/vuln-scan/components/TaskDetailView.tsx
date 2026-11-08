// 用于展示页面中的所有tabs
"use client";

import { normalizeTCPHeuristicConfig } from '@/lib/tcp-heuristic'
import { useState, useEffect, useRef, type WheelEvent } from "react";
import { DetailCategoryContext } from './DetailResultToolbar';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
	import {
	  Select,
	  SelectContent,
	  SelectItem,
	  SelectTrigger,
	  SelectValue,
	} from "@/components/ui/select";
import { MoreHorizontal, ChevronDown, ArrowLeft, Globe, Server, Shield, Info, FileText, Copy, Fingerprint, Bug, BugIcon, Trees, FileChartPie, FolderTree, FolderSearch, Pencil, Check, X, RefreshCw, Target, FileCode, Layers, Captions, Bot } from "lucide-react";
import { FingerprintTable } from "./FingerprintTable";
import { ScanStageTooltipContent } from "./ScanStageTooltipContent";
import { FingerprintCardView } from "./FingerprintCardView";
import { PortScanResultTable } from "./PortScanResultTable";
import { VulnerabilityView } from "./VulnerabilityView";
import { useRouter } from "next/navigation";
import { scanFindingDetailHref } from "@/lib/scan-finding-detail";
import { JsScanVulnerabilityTable, type JsScanVulnerability } from "./JsScanVulnerabilityTable";
import { JsScanAssetTable } from "./JsScanAssetTable";
import { JsScanInterfaceTable } from "./JsScanInterfaceTable";
import { JsScanSiteTree } from "./JsScanSiteTree";
import { DirScanResultTable } from "./DirScanResultTable";
import { SubdomainResultsPanel } from "./SubdomainResultsPanel";
import { getStatusBadge as renderTaskStatusBadge } from "@/lib/status-badge";
import type {
  WebScanTask,
  WebScanTaskSummary,
  PortInfo,
  SubdomainScanConfig,
  WebScanConfig,
  PortScanConfig,
  CrackScanConfig,
  JsScanConfig,
  ScheduleConfig,
  TaskScheduleLog,
  VulnerabilityInfo,
} from "../types";
import { fetchWithAuth } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { useIsMobile } from "@/hooks/use-mobile";
import { copyToClipboard } from "@/lib/clipboard";
import toast from "react-hot-toast";
import { formatSchedule, formatScheduleTimestamp, scheduleFromTask } from "../utils/task-schedule";
import { TaskScheduleSettings } from "../new/components/TaskScheduleSettings";
import { createDefaultSchedule } from "../utils/task-schedule";

interface TaskDetailViewProps {
  tasks: WebScanTaskSummary[];
  currentTaskId: string | null;
  currentTab?: string; // 当前选中的 tab
  version?: number; // 指定版本号，如果提供则只显示该版本的数据
  onBack: () => void;
  onTaskChange: (taskId: string, tab?: string) => void;
  onTabChange?: (tab: string) => void; // tab 切换回调
}

const normalizeScanType = (scanType?: string | null): string => {
  const normalized = String(scanType || "").toLowerCase().trim();
  if (normalized === "combination_task" || normalized === "combination-task") {
    return "combination";
  }
  return normalized || "webscan";
};

const taskToScheduleConfig = (task: WebScanTask | null): ScheduleConfig => {
  if (!task) return createDefaultSchedule();
  return scheduleFromTask(task);
};

const combinationTabGroups = {
  overview: ["info"],
  findings: ["vulnerability", "dirresults", "vulnerabilities"],
  assets: ["subdomains", "port", "fingerprint", "sitetree", "interfaces", "assets"],
} as const;

type CombinationTabGroup = keyof typeof combinationTabGroups;

function AICreatedMark({ label }: { label: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          role="img"
          aria-label={label}
          tabIndex={0}
          className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground/70 hover:text-muted-foreground focus-visible:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Bot className="h-3.5 w-3.5" aria-hidden="true" />
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export function TaskDetailView({
  tasks,
  currentTaskId,
  currentTab = "info",
  version,
  onBack,
  onTaskChange,
  onTabChange,
}: TaskDetailViewProps) {
  const { t, language } = useI18n();
  const router = useRouter();
  const taskStatusLabels = {
    pending: t.vulnScan.statuses.pending,
    running: t.vulnScan.statuses.running,
    pausing: t.vulnScan.statuses.pausing,
    paused: t.vulnScan.statuses.paused,
    recovering: t.vulnScan.statuses.recovering,
    deleting: t.vulnScan.statuses.deleting,
    completed: t.vulnScan.statuses.completed,
    failed: t.vulnScan.statuses.failed,
    cancelled: t.vulnScan.statuses.cancelled,
  };
  const [activeTab, setActiveTab] = useState(currentTab);
  const [currentTask, setCurrentTask] = useState<WebScanTask | null>(null);

  const [ports, setPorts] = useState<PortInfo[]>([]);
  const [portTabCount, setPortTabCount] = useState<number | null>(null);
  const [fingerprintLayout, setFingerprintLayout] = useState<"table" | "card">(
    "card"
  );
  const isMobile = useIsMobile();
  const effectiveFingerprintLayout = isMobile ? "card" : fingerprintLayout;
  const [pollingTimer, setPollingTimer] = useState<NodeJS.Timeout | null>(null);
  const pollingTimerRef = useRef<NodeJS.Timeout | null>(null);
  const [showAllTargets, setShowAllTargets] = useState(false);
  const [fingerprintTabCount, setFingerprintTabCount] = useState<number | null>(null);
  const [vulnerabilityTabCount, setVulnerabilityTabCount] = useState<number | null>(null);
  const isMountedRef = useRef(true);
  // 用于防止重复请求
  const loadingRef = useRef<{
    detail: boolean;
    results: boolean;
    scheduleLogs: boolean;
  }>({ detail: false, results: false, scheduleLogs: false });
  const lastTaskIdRef = useRef<string | null>(null);
  const lastResultStatsKeyRef = useRef<string | null>(null);

  // JS 扫描相关 state
  const [jsScanVulnCount, setJsScanVulnCount] = useState<number>(0);
  const [jsScanAssetCount, setJsScanAssetCount] = useState<number>(0);
  const [jsScanSiteTreeCount, setJsScanSiteTreeCount] = useState<number>(0);
  const [jsScanInterfaceCount, setJsScanInterfaceCount] = useState<number>(0);
  
  // 目录扫描相关 state
  const [dirScanResultCount, setDirScanResultCount] = useState<number>(0);
  const [subdomainTabCount, setSubdomainTabCount] = useState<number | null>(null);
  
  // 任务名称编辑相关 state
  const [isEditingName, setIsEditingName] = useState(false);
  const [editedName, setEditedName] = useState("");
  const [taskSwitcherOpen, setTaskSwitcherOpen] = useState(false);
  const [taskSearch, setTaskSearch] = useState("");
  const switcherTitleRef = useRef<HTMLHeadingElement>(null);
  const [scheduleDialogOpen, setScheduleDialogOpen] = useState(false);
  const [scheduleDraft, setScheduleDraft] = useState<ScheduleConfig>(() => createDefaultSchedule());
  const [savingSchedule, setSavingSchedule] = useState(false);
  const [scheduleLogs, setScheduleLogs] = useState<TaskScheduleLog[]>([]);
  const [loadingScheduleLogs, setLoadingScheduleLogs] = useState(false);
  const currentScanType = normalizeScanType(currentTask?.scanType);
  const effectiveVersion = version ?? currentTask?.version;

  const openWebFinding = (vuln: VulnerabilityInfo) => {
    const scanVersion = vuln.version ?? effectiveVersion;
    if (!scanVersion || !currentTaskId) return;
    router.push(scanFindingDetailHref({
      source: 'web', nativeId: vuln.id, taskId: vuln.taskId || currentTaskId, version: scanVersion,
      templateId: vuln.templateId, protocol: vuln.protocol, url: vuln.vulnUrl,
      name: vuln.vulnName, taskName: currentTask?.taskName,
    }, window.location.pathname + window.location.search));
  };

  const openJsFinding = (vuln: JsScanVulnerability) => {
    const scanVersion = vuln.version ?? effectiveVersion;
    if (!scanVersion || !currentTaskId) return;
    router.push(scanFindingDetailHref({
      source: 'js', taskId: vuln.taskId || currentTaskId, version: scanVersion,
      nativeId: vuln.id, name: vuln.title, taskName: currentTask?.taskName, createdAt: vuln.createdAt,
    }, window.location.pathname + window.location.search));
  };

  const handleTabsWheel = (event: WheelEvent<HTMLDivElement>) => {
    const container = event.currentTarget;
    const maxScrollLeft = container.scrollWidth - container.clientWidth;

    if (maxScrollLeft <= 0 || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) {
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    container.scrollLeft = Math.max(
      0,
      Math.min(maxScrollLeft, container.scrollLeft + event.deltaY)
    );
  };

  const getCombinationModuleMeta = (moduleId: string) => {
    switch (String(moduleId || "").toLowerCase()) {
      case "subdomain":
        return { label: t.vulnScan.taskManagement.subdomain, icon: Captions, tone: "text-tone-green" };
      case "portscan":
        return { label: t.vulnScan.taskManagement.portscan, icon: Server, tone: "text-emerald-500 dark:text-emerald-400" };
      case "webscan":
        return { label: t.vulnScan.taskManagement.webscan, icon: Globe, tone: "text-blue-500 dark:text-blue-400" };
      case "dirscan":
        return { label: t.vulnScan.taskManagement.dirscan, icon: FolderSearch, tone: "text-purple-500 dark:text-purple-400" };
      case "jsscan":
        return { label: t.vulnScan.taskManagement.jsscan, icon: FileCode, tone: "text-amber-500 dark:text-amber-400" };
      case "crack":
        return { label: t.vulnScan.taskManagement.crack, icon: Target, tone: "text-rose-500 dark:text-rose-400" };
      default:
        return { label: moduleId || t.vulnScan.taskDetail.unknownModule, icon: Shield, tone: "text-muted-foreground" };
    }
  };

  const formatTaskSchedule = (task: WebScanTask | null) => {
    if (!task || !task.scheduleSpec || task.scheduleEnabled === false) {
      return task && normalizeScanType(task.scanType) === "combination" ? t.vulnScan.newScan.scheduleModes.manual : null;
    }
    return formatSchedule(task.scheduleSpec, t);
  };

  const scheduleSummary = formatTaskSchedule(currentTask);
  const latestScheduleLog = scheduleLogs[0];

  const activeCombinationTabGroup: CombinationTabGroup =
    combinationTabGroups.findings.some((tab) => tab === activeTab)
      ? "findings"
      : combinationTabGroups.assets.some((tab) => tab === activeTab)
        ? "assets"
        : "overview";

  const handleCombinationTabGroupChange = (group: CombinationTabGroup) => {
    if (group === activeCombinationTabGroup) return;

    let nextTab = "info";
    if (group === "findings") {
      nextTab = jsScanVulnCount > 0
        ? "vulnerabilities"
        : dirScanResultCount > 0
          ? "dirresults"
          : "vulnerability";
    }
    if (group === "assets") {
      nextTab = jsScanAssetCount > 0
        ? "assets"
        : fingerprintTabCount || currentTask?.fingerprintCount
          ? "fingerprint"
          : subdomainTabCount || currentTask?.subdomainCount
            ? "subdomains"
            : "port";
    }

    setActiveTab(nextTab);
    onTabChange?.(nextTab);
  };

  const combinationFlowStages = (() => {
    if (currentScanType !== "combination" || !currentTask) {
      return [] as Array<{ stageOrder: number; stageIndex: number; nodes: Array<{ key: string; moduleId: string }> }>;
    }

    const config = (currentTask.config as any) || {};
    const rawNodes = Array.isArray(config.combinationNodes)
      ? config.combinationNodes
      : Array.isArray(config.nodes)
        ? config.nodes
        : [];

    if (!Array.isArray(rawNodes) || rawNodes.length === 0) {
      return [] as Array<{ stageOrder: number; stageIndex: number; nodes: Array<{ key: string; moduleId: string }> }>;
    }

    const grouped = new Map<number, Array<{ key: string; moduleId: string; laneIndex: number; order: number }>>();

    rawNodes.forEach((node: any, order: number) => {
      const moduleId = String(node?.moduleId || "").trim();
      if (!moduleId) return;
      const stageIndex = Number.isFinite(Number(node?.stageIndex)) ? Number(node.stageIndex) : 0;
      const laneIndex = Number.isFinite(Number(node?.laneIndex)) ? Number(node.laneIndex) : 0;
      if (!grouped.has(stageIndex)) grouped.set(stageIndex, []);
      grouped.get(stageIndex)!.push({
        key: `${moduleId}-${stageIndex}-${laneIndex}-${order}`,
        moduleId,
        laneIndex,
        order,
      });
    });

    return Array.from(grouped.entries())
      .sort((a, b) => a[0] - b[0])
      .map(([stageIndex, nodes], stageOrder) => ({
        stageOrder,
        stageIndex,
        nodes: [...nodes]
          .sort((a, b) => (a.laneIndex - b.laneIndex) || (a.order - b.order))
          .map((node) => ({ key: node.key, moduleId: node.moduleId })),
      }));
  })();

  // 加载任务详情
  const loadTaskDetail = async (taskId: string) => {
    // 防止重复请求
    if (loadingRef.current.detail) {
      return;
    }
    loadingRef.current.detail = true;

    try {
      // 如果指定了版本，在请求中带上版本参数
      const versionParam = version ? `?version=${version}` : "";
      const response = await fetchWithAuth(
        `/api/v1/webscan/task/${taskId}${versionParam}`
      );
      if (!response.ok) {
        return;
      }

      const data = await response.json();
      if (data.success && data.data && isMountedRef.current) {
        const task = data.data;
        setCurrentTask(task);

        // 如果任务已完成，停止轮询
        if (
          task.status === "completed" ||
          task.status === "failed" ||
          task.status === "cancelled"
        ) {
          if (pollingTimerRef.current) {
            clearInterval(pollingTimerRef.current);
            pollingTimerRef.current = null;
            setPollingTimer(null);
          }
        }
      }
    } catch (error) {
      console.error("Load task detail error:", error);
    } finally {
      loadingRef.current.detail = false;
    }
  };

  // 加载任务结果
  const loadTaskResults = async (taskId: string, options?: { force?: boolean }) => {
    if (!isMountedRef.current) return;

    // 防止重复请求
    if (loadingRef.current.results) {
      return;
    }
    loadingRef.current.results = true;

    try {
      // 优先使用已有任务信息确定任务类型，避免结果请求内部再次打详情接口
      let scanType = "";

      if (currentTask && currentTask.id === taskId) {
        scanType = normalizeScanType(currentTask.scanType);
      } else {
        const task = tasks.find((t) => t.id === taskId);
        if (task) {
          scanType = normalizeScanType(task.scanType);
        }
      }

      if (!scanType) {
        return;
      }

      const resultStatsKey = `${taskId}:${effectiveVersion ?? "latest"}:${scanType}`;
      if (!options?.force && lastResultStatsKeyRef.current === resultStatsKey) {
        return;
      }
      lastResultStatsKeyRef.current = resultStatsKey;

      if (scanType === "webscan") {
        // 网站扫描：清空端口数据并预加载结果数量
        if (isMountedRef.current) {
          setPorts([]);
          setPortTabCount(null);
        }
        await loadWebScanResultStats(taskId, effectiveVersion);
      } else if (scanType === "jsscan") {
        // JS 扫描：加载 JS 扫描数据
        if (isMountedRef.current) {
          setPorts([]);
          setPortTabCount(null);
        }
        await loadJsScanResultStats(taskId, effectiveVersion);
      } else if (scanType === "dirscan") {
        // 目录扫描：加载目录扫描结果数量
        if (isMountedRef.current) {
          setPorts([]);
          setPortTabCount(null);
        }
        await loadDirScanResultCount(taskId, effectiveVersion);
      } else if (scanType === "subdomain") {
        if (isMountedRef.current) {
          setPorts([]);
          setPortTabCount(null);
        }
        await loadSubdomainResultCount(taskId);
      } else if (scanType === "portscan" || scanType === "crack") {
        if (isMountedRef.current) {
          setPorts([]);
        }
      } else if (scanType === "combination") {
        // 组合任务：开放全部标签，加载多维度结果
        if (isMountedRef.current) {
          setPorts([]);
        }
        await Promise.all([
          loadSubdomainResultCount(taskId),
          loadWebScanResultStats(taskId, effectiveVersion),
          loadJsScanResultStats(taskId, effectiveVersion),
          loadDirScanResultCount(taskId, effectiveVersion),
        ]);
      }
    } catch (error) {
      console.error("Load task results error:", error);
    } finally {
      loadingRef.current.results = false;
    }
  };

  // 预加载网站扫描结果数量，避免必须切换到对应 tab 才显示正确数量
  const loadWebScanResultStats = async (taskId: string, version?: number) => {
    if (!isMountedRef.current) return;

    try {
      const versionParam = version ? `&version=${version}` : "";
      const [fingerprintResponse, vulnerabilityResponse] = await Promise.all([
        fetchWithAuth(`/api/v1/webscan/task/${taskId}/fingerprints?page=1&pageSize=1${versionParam}`),
        fetchWithAuth(`/api/v1/webscan/task/${taskId}/vulnerabilities?page=1&pageSize=1${versionParam}`),
      ]);

      if (!isMountedRef.current) return;

      let fingerprintCount = 0;
      let vulnerabilityCount = 0;

      if (fingerprintResponse.ok) {
        const fingerprintData = await fingerprintResponse.json();
        if (fingerprintData.success && fingerprintData.data) {
          fingerprintCount = fingerprintData.data.total || 0;
        }
      }

      if (vulnerabilityResponse.ok) {
        const vulnerabilityData = await vulnerabilityResponse.json();
        if (vulnerabilityData.success && vulnerabilityData.data) {
          vulnerabilityCount = vulnerabilityData.data.total || 0;
        }
      }

      if (isMountedRef.current) {
        setFingerprintTabCount(fingerprintCount);
        setVulnerabilityTabCount(vulnerabilityCount);
      }
    } catch (error) {
      console.error("Failed to load web scan stats:", error);
    }
  };

  // 加载 JS 扫描结果统计（避免在详情页一次性拉取全量数据）
  const loadJsScanResultStats = async (taskId: string, version?: number) => {
    if (!isMountedRef.current) return;

    try {
      const versionParam = version ? `&version=${version}` : "";
      const loadPagedTotal = async (
        url: string,
        apply: (count: number) => void,
      ) => {
        const response = await fetchWithAuth(url);
        if (!response.ok || !isMountedRef.current) return;

        const data = await response.json();
        if (!isMountedRef.current) return;
        if (data.success && data.data) {
          apply(data.data.total || 0);
        }
      };

      await Promise.allSettled([
        loadPagedTotal(
          `/api/v1/jsscan/vulnerabilities/${taskId}?page=1&pageSize=1${versionParam}`,
          (count) => setJsScanVulnCount(count),
        ),
        loadPagedTotal(
          `/api/v1/jsscan/assets/${taskId}?page=1&pageSize=1${versionParam}`,
          (count) => setJsScanAssetCount(count),
        ),
        loadPagedTotal(
          `/api/v1/jsscan/sitetree/${taskId}?page=1&pageSize=1${versionParam}`,
          (count) => setJsScanSiteTreeCount(count),
        ),
        loadPagedTotal(
          `/api/v1/jsscan/interfaces/${taskId}?page=1&pageSize=1${versionParam}`,
          (count) => setJsScanInterfaceCount(count),
        ),
      ]);
    } catch (error) {
      console.error("Failed to load JS scan stats:", error);
    }
  };

  // 加载目录扫描结果数量
  const loadDirScanResultCount = async (taskId: string, version?: number) => {
    if (!isMountedRef.current) return;

    try {
      const versionParam = version ? `&version=${version}` : "";
      const response = await fetchWithAuth(
        `/api/v1/webscan/task/${taskId}/dir-results?limit=1&offset=0${versionParam}`
      );

      if (!isMountedRef.current) return;

      if (response.ok) {
        const data = await response.json();
        if (data.success && data.data && isMountedRef.current) {
          setDirScanResultCount(data.data.total || 0);
        }
      }
    } catch (error) {
      console.error("Failed to load dir scan result count:", error);
    }
  };

  const loadSubdomainResultCount = async (taskId: string) => {
    if (!isMountedRef.current) return;

    try {
      const response = await fetchWithAuth(
        `/api/v1/subdomain/task/${taskId}/results?limit=1&offset=0`
      );

      if (!isMountedRef.current) return;

      if (response.ok) {
        const data = await response.json();
        if (data.success && data.data && isMountedRef.current) {
          setSubdomainTabCount(data.data.total || 0);
        }
      }
    } catch (error) {
      console.error("Failed to load subdomain result count:", error);
    }
  };

  const loadScheduleLogs = async (taskId: string) => {
    if (!isMountedRef.current || loadingRef.current.scheduleLogs) return;
    loadingRef.current.scheduleLogs = true;
    setLoadingScheduleLogs(true);
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/schedule/logs?limit=5`);
      if (!response.ok || !isMountedRef.current) return;

      const data = await response.json();
      if (data.success && data.data && isMountedRef.current) {
        setScheduleLogs(Array.isArray(data.data.items) ? data.data.items : []);
      }
    } catch (error) {
      console.error("Failed to load schedule logs:", error);
    } finally {
      loadingRef.current.scheduleLogs = false;
      if (isMountedRef.current) {
        setLoadingScheduleLogs(false);
      }
    }
  };

  // 组件挂载和卸载时更新 ref
  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  // 同步 currentTab 到 activeTab（当 URL 参数变化时）
  useEffect(() => {
    if (currentTab) {
      const timer = window.setTimeout(() => {
        setActiveTab(currentTab);
      }, 0);

      return () => window.clearTimeout(timer);
    }
  }, [currentTab]);

  // 轮询任务状态和结果
  useEffect(() => {
    const clearPollingTimer = () => {
      if (pollingTimerRef.current) {
        clearInterval(pollingTimerRef.current);
        pollingTimerRef.current = null;
        setPollingTimer(null);
      }
    };

    const timer = window.setTimeout(() => {
      if (!currentTaskId) {
        if (!isMountedRef.current) {
          return;
        }
        setCurrentTask(null);
        setPorts([]);
        setPortTabCount(null);
        // fingerprints and vulnerabilities are now managed by their respective components
        // 清理 JS 扫描数据统计
        setJsScanVulnCount(0);
        setJsScanAssetCount(0);
        setJsScanSiteTreeCount(0);
        setJsScanInterfaceCount(0);

        setFingerprintTabCount(null);
        setVulnerabilityTabCount(null);
        setSubdomainTabCount(null);
        setScheduleLogs([]);
        setScheduleDialogOpen(false);
        // 清理目录扫描数据
        setDirScanResultCount(0);
        clearPollingTimer();
        lastTaskIdRef.current = null;
        lastResultStatsKeyRef.current = null;
        // 重置加载状态
        loadingRef.current.detail = false;
        loadingRef.current.results = false;
        loadingRef.current.scheduleLogs = false;
        return;
      }

      // 如果任务ID和版本都没有变化，不重新加载（避免 tasks 数组引用变化导致的重复请求）
      const currentKey = `${currentTaskId}-${version || "latest"}`;
      if (lastTaskIdRef.current === currentKey) {
        return;
      }
      lastTaskIdRef.current = currentKey;
      lastResultStatsKeyRef.current = null;
      setSubdomainTabCount(null);
      setScheduleLogs([]);
      setScheduleDialogOpen(false);

      // 重置加载状态，允许重新加载
      loadingRef.current.detail = false;
      loadingRef.current.results = false;
      loadingRef.current.scheduleLogs = false;

      // 立即加载一次
      void loadTaskDetail(currentTaskId);
      void loadScheduleLogs(currentTaskId);
      // 同时加载结果数据（不依赖任务详情，因为可以从 tasks 或直接请求获取 scanType）
      void loadTaskResults(currentTaskId);

      // 清除之前的定时器
      clearPollingTimer();

      // 如果任务正在运行、待处理、恢复中或删除中，开始轮询
      // 优先使用 tasks 数组中的状态，如果没有则等待 loadTaskDetail 的结果
      const task = tasks.find((t) => t.id === currentTaskId);
      const taskStatus = task?.status || currentTask?.status;
      const shouldPoll = taskStatus === "running" || taskStatus === "pending" || taskStatus === "recovering" || taskStatus === "deleting";

      if (shouldPoll) {
        const pollingTimer = setInterval(() => {
          if (!isMountedRef.current) {
            clearInterval(pollingTimer);
            pollingTimerRef.current = null;
            return;
          }

          // 加载任务详情和结果（loadTaskDetail 内部会检查状态并停止轮询）
          void loadTaskDetail(currentTaskId);
          void loadTaskResults(currentTaskId, { force: true });
        }, 2000); // 每2秒更新一次

        pollingTimerRef.current = pollingTimer;
        setPollingTimer(pollingTimer);
      }
    }, 0);

    return () => {
      window.clearTimeout(timer);
      clearPollingTimer();
    };
    // 依赖 currentTaskId 和 version，不依赖 tasks 数组（避免 tasks 引用变化导致的重复请求）
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentTaskId, version]);

  const currentTaskStatus = currentTask?.status;

  // 根据 currentTask 状态动态控制轮询（当任务状态变化时）
  useEffect(() => {
    if (!currentTaskStatus || !currentTaskId) return;

    const isRunning =
      currentTaskStatus === "running" || currentTaskStatus === "pending";
    const hasPolling = pollingTimerRef.current !== null;

    // 如果任务已完成但还在轮询，停止轮询
    if (!isRunning && hasPolling) {
      if (pollingTimerRef.current) {
        clearInterval(pollingTimerRef.current);
        pollingTimerRef.current = null;
        setPollingTimer(null);
      }
    }
    // 注意：启动轮询的逻辑在第一个 useEffect 中处理，这里只负责停止
  }, [currentTaskStatus, currentTaskId]);

  // 当 currentTask 的 scanType 确定后，补一次结果预加载，避免首次进入详情页时因 scanType 未知导致数量延后到点 tab 才出现
  useEffect(() => {
    if (!currentTask || !currentTaskId) return;

    const timer = window.setTimeout(() => {
      void loadTaskResults(currentTaskId);
    }, 0);

    return () => window.clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentTask?.scanType, currentTaskId]);

  // 组件卸载时清理定时器
  useEffect(() => {
    return () => {
      isMountedRef.current = false;
      if (pollingTimerRef.current) {
        clearInterval(pollingTimerRef.current);
        pollingTimerRef.current = null;
        setPollingTimer(null);
      }
    };
  }, [pollingTimer]);

  // 更新任务名称
  const handleUpdateName = async () => {
    if (!currentTask || !editedName.trim()) return;

    const loadingToast = toast.loading(t.common.updating);

    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${currentTask.id}/name`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ taskName: editedName.trim() }),
      });

      if (response.ok) {
        setCurrentTask({ ...currentTask, taskName: editedName.trim() });
        setIsEditingName(false);
        toast.success(t.common.updateSuccess, { id: loadingToast });
      } else {
        const data = await response.json();
        throw new Error(data.error || t.common.updateFailed);
      }
    } catch (error: any) {
      toast.error(error.message || t.common.updateFailed, { id: loadingToast });
    }
  };

  const openScheduleEditor = () => {
    if (!currentTask) return;
    setScheduleDraft(taskToScheduleConfig(currentTask));
    setScheduleDialogOpen(true);
  };

  const applyScheduleResponse = (payload: any) => {
    if (!currentTask) return;
    setCurrentTask({
      ...currentTask,
      scheduleSpec: payload?.scheduleSpec ?? null,
      scheduleEnabled: payload?.scheduleEnabled,
      nextRunAt: payload?.nextRunAt,
    });
  };

  const saveSchedule = async (nextSchedule: ScheduleConfig, successMessage: string, failureMessage: string) => {
    if (!currentTask) return;

    const loadingToast = toast.loading(t.common.updating);
    setSavingSchedule(true);
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${currentTask.id}/schedule`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ schedule: nextSchedule }),
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok || data.success === false) {
        throw new Error(data.error || failureMessage);
      }

      applyScheduleResponse(data.data || {});
      setScheduleDialogOpen(false);
      await loadScheduleLogs(currentTask.id);
      toast.success(successMessage, { id: loadingToast });
    } catch (error: any) {
      toast.error(error.message || failureMessage, { id: loadingToast });
    } finally {
      setSavingSchedule(false);
    }
  };

  const handleSaveSchedule = () => {
    void saveSchedule(
      scheduleDraft,
      t.vulnScan.taskDetail.scheduleUpdateSuccess,
      t.vulnScan.taskDetail.scheduleUpdateFailed,
    );
  };

  const handleDisableSchedule = () => {
    void saveSchedule(
      { enabled: false, spec: null },
      t.vulnScan.taskDetail.scheduleDisableSuccess,
      t.vulnScan.taskDetail.scheduleDisableFailed,
    );
  };

  // 复制到剪贴板
  const handleCopyToClipboard = (text: string) => {
    copyToClipboard(text);
    toast.success(t.common.copiedToClipboard);
  };

  if (!currentTaskId || !currentTask) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-muted-foreground">{t.vulnScan.taskDetail.selectTask}</p>
      </div>
    );
  }

  const getStatusBadge = (status: string) => renderTaskStatusBadge(status, taskStatusLabels);

  const formatDate = (dateString?: string) => {
    if (!dateString) return "-";
    return formatScheduleTimestamp(dateString, language === "zh" ? "zh-CN" : "en-US");
  };

  const scheduleEnabled = Boolean(currentTask.scheduleSpec && currentTask.scheduleEnabled !== false);
  const alarmLabels = t.vulnScan.newScan.alarmSchedule;
  const onceStateLabels: Partial<Record<TaskScheduleLog["status"], string>> = { success: alarmLabels.triggered, failed: alarmLabels.failed, expired: alarmLabels.expired, skipped: alarmLabels.skipped, dispatching: alarmLabels.dispatching };
  const scheduleStateText = currentTask.scheduleSpec?.kind === "once" && latestScheduleLog
    ? onceStateLabels[latestScheduleLog.status] || (scheduleEnabled ? t.vulnScan.taskDetail.scheduleEnabled : t.vulnScan.taskDetail.scheduleDisabled)
    : scheduleEnabled ? t.vulnScan.taskDetail.scheduleEnabled : t.vulnScan.taskDetail.scheduleDisabled;
  const logStatusLabels: Record<TaskScheduleLog["status"], string> = { success: t.vulnScan.taskDetail.scheduleStatusSuccess, failed: alarmLabels.failed, dispatching: alarmLabels.dispatching, skipped: alarmLabels.skipped, expired: alarmLabels.expired };
  const scheduleLogReason = (item: TaskScheduleLog) => item.status === "expired" ? alarmLabels.missedReason : item.status === "skipped" ? alarmLabels.busyReason : item.errorMsg;
  const canEditSchedule = version === undefined && currentTask.status !== "deleting";
  const webVulnerabilityCount = vulnerabilityTabCount ?? (currentTask.vulnCount || 0);
  const subdomainCount = subdomainTabCount ?? currentTask.subdomainCount ?? currentTask.resultCount ?? 0;
  const portCount = portTabCount ?? currentTask.portCount ?? ports.length ?? 0;
  const fingerprintCount = fingerprintTabCount ?? (currentTask.fingerprintCount || 0);
  const findingsTotal =
    webVulnerabilityCount +
    dirScanResultCount +
    jsScanVulnCount;
  const assetsTotal =
    subdomainCount +
    portCount +
    fingerprintCount +
    jsScanSiteTreeCount +
    jsScanInterfaceCount +
    jsScanAssetCount;

  const combinationGroupItems = [
    {
      group: "overview" as const,
      label: t.vulnScan.taskDetail.overview,
      icon: Info,
      count: null,
    },
    {
      group: "assets" as const,
      label: t.vulnScan.taskDetail.assetsGroup,
      icon: Layers,
      count: assetsTotal,
    },
    {
      group: "findings" as const,
      label: t.vulnScan.taskDetail.findings,
      icon: Bug,
      count: findingsTotal,
    },
  ];

  const combinationFindingTabs = [
    {
      value: "vulnerability",
      label: t.vulnScan.taskDetail.vulnInfo,
      icon: Bug,
      count: webVulnerabilityCount,
    },
    {
      value: "dirresults",
      label: t.vulnScan.taskDetail.dirResults,
      icon: FolderSearch,
      count: dirScanResultCount,
    },
    {
      value: "vulnerabilities",
      label: t.vulnScan.taskDetail.jsVulnInfo,
      icon: BugIcon,
      count: jsScanVulnCount,
    },
  ];

  const combinationAssetTabs = [
    {
      value: "subdomains",
      label: t.vulnScan.taskManagement.subdomain,
      icon: Captions,
      count: subdomainCount,
    },
    {
      value: "port",
      label: t.vulnScan.taskDetail.portInfo,
      icon: Server,
      count: portCount,
    },
    {
      value: "fingerprint",
      label: t.vulnScan.taskDetail.fingerprintInfo,
      icon: Fingerprint,
      count: fingerprintCount,
    },
    {
      value: "sitetree",
      label: t.vulnScan.taskDetail.siteTree,
      icon: Trees,
      count: jsScanSiteTreeCount,
    },
    {
      value: "interfaces",
      label: t.vulnScan.taskDetail.interfaces,
      icon: FileCode,
      count: jsScanInterfaceCount,
    },
    {
      value: "assets",
      label: t.vulnScan.taskDetail.assets,
      icon: FileChartPie,
      count: jsScanAssetCount,
    },
  ];

  const activeCombinationResultTabs =
    activeCombinationTabGroup === "findings"
      ? combinationFindingTabs
      : activeCombinationTabGroup === "assets"
        ? combinationAssetTabs
        : [];

  // 获取可切换的任务列表（按创建时间倒序）
  const availableTasks = tasks
    .filter((t) => t.id !== currentTaskId)
    .sort(
      (a, b) =>
        new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    );

  const mobileCategory = isMobile && activeCombinationResultTabs.length > 0 ? (
    <Select value={activeTab} onValueChange={value => { setActiveTab(value); onTabChange?.(value) }}>
      <SelectTrigger className="h-9 w-full min-w-0 px-2 text-xs [&_[data-slot=select-value]]:min-w-0 [&_[data-slot=select-value]]:truncate" aria-label={activeCombinationTabGroup === "findings" ? t.vulnScan.taskDetail.findings : t.vulnScan.taskDetail.assetsGroup}><SelectValue /></SelectTrigger>
      <SelectContent>{activeCombinationResultTabs.map(item => <SelectItem key={item.value} value={item.value}>{item.label} ({item.count})</SelectItem>)}</SelectContent>
    </Select>
  ) : null;

  return (
    <div className="flex min-w-0 flex-col h-full">
      {/* 页头 */}
      {isMobile && <div className="flex min-w-0 shrink-0 items-center gap-1 border-b bg-card px-3 py-2" data-slot="mobile-detail-header">
        <Button variant="ghost" size="icon" className="h-9 w-9 shrink-0" aria-label={t.vulnScan.backToList} onClick={onBack}><ArrowLeft className="h-4 w-4" /></Button>
        <Button variant="ghost" className="h-9 min-w-0 flex-1 justify-start px-2" aria-label={t.vulnScan.taskDetail.switchTask} onClick={() => { setTaskSearch(''); setTaskSwitcherOpen(true) }}>
          {currentTask.createdByAI && (
            <span role="img" aria-label={t.vulnScan.taskManagement.aiCreated} className="mr-1.5 inline-flex shrink-0 text-muted-foreground/70">
              <Bot className="h-4 w-4" aria-hidden="true" />
            </span>
          )}
          <span className="truncate">{currentTask.taskName || currentTask.id}</span><ChevronDown className="ml-auto h-4 w-4 shrink-0" />
        </Button>
        <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="h-9 w-9 shrink-0 text-lg" aria-label={t.common.action}><MoreHorizontal className="h-4 w-4" /></Button></DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => { setEditedName(currentTask.taskName || ''); setIsEditingName(true) }}>{t.vulnScan.taskManagement.editTaskName}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => handleCopyToClipboard(currentTaskId || '')}>{t.mobileList.copyTaskId}</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>}
      {isMobile && <Sheet open={taskSwitcherOpen} onOpenChange={setTaskSwitcherOpen}>
        <SheetContent side="bottom" aria-describedby={undefined} className="max-h-[85dvh] gap-0 rounded-t-2xl pb-[max(1rem,env(safe-area-inset-bottom))]" onOpenAutoFocus={event => { event.preventDefault(); switcherTitleRef.current?.focus() }}>
          <SheetHeader className="pr-10"><SheetTitle ref={switcherTitleRef} tabIndex={-1} className="outline-none">{t.vulnScan.taskDetail.switchTask}</SheetTitle></SheetHeader>
          <div className="px-4 pb-3"><Input aria-label={t.common.search} placeholder={t.vulnScan.taskManagement.searchPlaceholder} value={taskSearch} onChange={event => setTaskSearch(event.target.value)} className="text-base" /></div>
          <div className="min-h-0 space-y-1 overflow-y-auto overscroll-contain px-4">
            {[currentTask, ...availableTasks].filter(task => `${task.taskName || ''} ${task.id}`.toLowerCase().includes(taskSearch.toLowerCase())).map(task => <Button key={task.id} variant="ghost" className="h-auto min-h-11 w-full justify-start whitespace-normal break-all px-3 py-3 text-left" aria-current={task.id === currentTaskId ? 'true' : undefined} onClick={() => { setTaskSwitcherOpen(false); if (task.id !== currentTaskId) onTaskChange(task.id, "info") }}><span className="min-w-0 flex-1">{task.taskName || task.id}</span>{task.id === currentTaskId && <Check className="ml-2 h-4 w-4 shrink-0" />}</Button>)}
          </div>
        </SheetContent>
      </Sheet>}
      {isMobile && <Dialog open={isEditingName} onOpenChange={setIsEditingName}>
        <DialogContent aria-describedby={undefined}><DialogHeader><DialogTitle>{t.vulnScan.taskManagement.editTaskName}</DialogTitle></DialogHeader>
          <Input aria-label={t.vulnScan.taskName} value={editedName} onChange={event => setEditedName(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') handleUpdateName() }} className="text-base" />
          <DialogFooter><Button variant="outline" onClick={() => setIsEditingName(false)}>{t.common.cancel}</Button><Button onClick={handleUpdateName}>{t.common.save}</Button></DialogFooter>
        </DialogContent>
      </Dialog>}
      {!isMobile && <div className="border-b border-border bg-card px-4 py-3">
        <div className="flex min-w-0 flex-col items-start gap-2 md:flex-row md:items-center md:justify-between">
          <Button variant="ghost" size="sm" className="h-11 shrink-0 md:h-8" onClick={onBack}>
            <ArrowLeft className="w-4 h-4 mr-2" />
            {t.vulnScan.backToList}
          </Button>
          <div className="flex w-full min-w-0 flex-col items-start gap-2 md:w-auto md:flex-row md:flex-wrap md:items-center md:justify-end md:gap-3">
            {/* 任务名称编辑 */}
            {isEditingName ? (
              <div className="flex min-w-0 max-w-full items-center gap-2">
                <Input
                  value={editedName}
                  onChange={(e) => setEditedName(e.target.value)}
                  className="h-11 min-w-0 flex-1 text-base md:h-8 md:w-48 md:text-sm"
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleUpdateName();
                    if (e.key === "Escape") setIsEditingName(false);
                  }}
                />
                <Button size="icon" variant="ghost" className="h-11 w-11 shrink-0 md:h-8 md:w-8" onClick={handleUpdateName}>
                  <Check className="h-4 w-4" />
                </Button>
                <Button size="icon" variant="ghost" className="h-11 w-11 shrink-0 md:h-8 md:w-8" onClick={() => setIsEditingName(false)}>
                  <X className="h-4 w-4" />
                </Button>
              </div>
            ) : (
              <div className="flex min-w-0 max-w-full items-center gap-2">
                {currentTask?.createdByAI && (
                  <AICreatedMark label={t.vulnScan.taskManagement.aiCreated} />
                )}
                <span className="min-w-0 break-all text-sm font-medium">
                  {currentTask?.taskName || currentTask?.id || t.vulnScan.taskDetail.untitledTask}
                </span>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-11 w-11 shrink-0 md:h-6 md:w-6"
                  aria-label={t.vulnScan.taskManagement.editTaskName}
                  onClick={() => {
                    setEditedName(currentTask?.taskName || "");
                    setIsEditingName(true);
                  }}
                >
                  <Pencil className="h-3 w-3" />
                </Button>
              </div>
            )}
            {/* 任务ID */}
            <div className="flex w-full min-w-0 items-center gap-1 text-xs text-muted-foreground md:w-auto">
              <span className="shrink-0">{t.vulnScan.taskDetail.taskIdLabel}:</span>
              <span className="min-w-0 break-all font-mono">{currentTaskId}</span>
              <Button
                size="icon"
                variant="ghost"
                className="h-11 w-11 shrink-0 md:h-5 md:w-5"
                aria-label={t.common.copy}
                onClick={() => handleCopyToClipboard(currentTaskId || "")}
              >
                <Copy className="h-3 w-3" />
              </Button>
            </div>
            {/* 任务选择器 */}
            <Select
              value={currentTaskId || ""}
              onValueChange={(value) => {
                onTaskChange(value);
              }}
            >
              <SelectTrigger className="h-11 w-full md:h-8 md:w-[200px]">
                <SelectValue>
                  {t.vulnScan.taskDetail.switchTask}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={currentTaskId || ""} disabled>
                  {t.vulnScan.taskDetail.currentTask}
                </SelectItem>
                {availableTasks.map((task) => (
                  <SelectItem key={task.id} value={task.id}>
                    <div className="flex min-w-0 max-w-full items-center gap-2">
                      {normalizeScanType(task.scanType) === "subdomain" ? (
                        <Captions className="h-3 w-3 shrink-0 text-tone-green" />
                      ) : normalizeScanType(task.scanType) === "webscan" ? (
                        <Globe className="w-3 h-3 text-blue-600 dark:text-blue-400" />
                      ) : normalizeScanType(task.scanType) === "portscan" ? (
                        <Server className="w-3 h-3 text-emerald-600 dark:text-emerald-400" />
                      ) : normalizeScanType(task.scanType) === "crack" ? (
                        <Target className="w-3 h-3 text-orange-600 dark:text-orange-400" />
                      ) : normalizeScanType(task.scanType) === "dirscan" ? (
                        <FolderSearch className="w-3 h-3 text-purple-600 dark:text-purple-400" />
	                      ) : normalizeScanType(task.scanType) === "jsscan" ? (
	                        <FileCode className="w-3 h-3 text-amber-600 dark:text-amber-400" />
	                      ) : normalizeScanType(task.scanType) === "combination" ? (
	                        <Layers className="w-3 h-3 text-slate-600 dark:text-slate-400" />
	                      ) : (
	                        <Shield className="w-3 h-3" />
	                      )}
                      <span>
                        {task.taskName || task.id}{" "}
                        (
                        {task.status === "running"
                          ? t.vulnScan.statuses.running
                          : task.status === "completed"
                            ? t.vulnScan.statuses.completed
                            : task.status}
                        )
                      </span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>}

      {/* 内容区域 */}
      <div className="relative min-w-0 flex-1 overflow-y-auto p-3 md:p-6">
        <Card className="min-w-0 p-3 md:p-6">
          <DetailCategoryContext.Provider value={mobileCategory}>
          <Tabs
            value={activeTab}
            onValueChange={(value) => {
              setActiveTab(value);
              onTabChange?.(value);
            }}
            className="w-full min-w-0"
          >
            <div
              className="heph-tabs-scrollbar w-full min-w-0 overflow-x-auto"
              onWheel={handleTabsWheel}
            >
              <TabsList
                className={currentScanType === "combination"
                  ? "h-auto w-full md:w-auto max-w-full flex-nowrap justify-start !gap-1 !rounded-lg !border-border/60 !bg-muted/35 !p-1"
                  : "h-auto min-w-max flex-nowrap justify-start"}
              >
                {currentScanType !== "combination" && (
                  <TabsTrigger value="info" className="flex-none items-center gap-2">
                    <Info className="w-4 h-4" />
                    {t.vulnScan.taskDetail.basicInfo}
                  </TabsTrigger>
                )}
                {currentScanType === "webscan" ? (
                  <>
                  <TabsTrigger
                    value="fingerprint"
                    className="flex-none items-center gap-2"
                  >
                    <Fingerprint className="w-4 h-4" />
                    {t.vulnScan.taskDetail.fingerprintInfo} ({fingerprintTabCount ?? (currentTask.fingerprintCount || 0)})
                  </TabsTrigger>
                  <TabsTrigger
                    value="vulnerability"
                    className="flex-none items-center gap-2"
                  >
                    <Bug className="w-4 h-4" />
                    {t.vulnScan.taskDetail.vulnInfo} ({vulnerabilityTabCount ?? (currentTask.vulnCount || 0)})
                  </TabsTrigger>
                </>
              ) : currentScanType === "portscan" ? (
                <>
                  <TabsTrigger value="port" className="flex-none items-center gap-2">
                    <Server className="w-4 h-4" />
                    {t.vulnScan.taskDetail.portInfo} ({portTabCount ?? currentTask.portCount ?? ports.length ?? 0})
                  </TabsTrigger>
                </>
              ) : currentScanType === "jsscan" ? (
                <>
                  <TabsTrigger
                    value="sitetree"
                    className="flex-none items-center gap-2"
                  >
                    <Trees className="w-4 h-4" />
                    {t.vulnScan.taskDetail.siteTree} ({jsScanSiteTreeCount})
                  </TabsTrigger>
                  <TabsTrigger value="interfaces" className="flex-none items-center gap-2">
                    <FileCode className="w-4 h-4" />
                    {t.vulnScan.taskDetail.interfaces} ({jsScanInterfaceCount})
                  </TabsTrigger>
                  <TabsTrigger
                    value="vulnerabilities"
                    className="flex-none items-center gap-2"
                  >
                    <Bug className="w-4 h-4" />
                    {t.vulnScan.taskDetail.jsVulnInfo} ({jsScanVulnCount})
                  </TabsTrigger>
                  <TabsTrigger
                    value="assets"
                    className="flex-none items-center gap-2"
                  >
                    <FileChartPie className="w-4 h-4" />
                    {t.vulnScan.taskDetail.assets} ({jsScanAssetCount})
                  </TabsTrigger>
                </>
              ) : currentScanType === "combination" ? (
                <>
                  {combinationGroupItems.map(({ group, label, icon: Icon, count }) => (
                    <button
                      key={group}
                      type="button"
                      role="tab"
                      aria-selected={activeCombinationTabGroup === group}
                      onClick={() => handleCombinationTabGroupChange(group)}
                      className={`flex h-8 min-w-0 flex-1 md:flex-none md:min-w-[104px] items-center justify-center gap-1 md:gap-1.5 whitespace-nowrap rounded-md border px-1.5 md:px-3 text-xs md:text-sm font-medium transition-[background-color,border-color,color,box-shadow] ${
                        activeCombinationTabGroup === group
                          ? "border-border bg-background text-foreground shadow-sm"
                          : "border-transparent text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                      }`}
                    >
                      <Icon className={`h-3.5 w-3.5 shrink-0 ${
                        activeCombinationTabGroup === group ? "text-primary" : ""
                      }`} />
                      <span>{label}</span>
                      {count !== null && (
                        <span className={`ml-0.5 rounded px-1.5 py-0.5 text-[10px] leading-none ${
                          activeCombinationTabGroup === group
                            ? "bg-primary/10 text-primary"
                            : "bg-muted text-muted-foreground"
                        }`}>
                          {count}
                        </span>
                      )}
                    </button>
                  ))}
                </>
              ) : currentScanType === "dirscan" ? (
                <>
                  <TabsTrigger
                    value="dirresults"
                    className="flex-none items-center gap-2"
                  >
                    <FolderSearch className="w-4 h-4" />
                    {t.vulnScan.taskDetail.dirResults} ({dirScanResultCount})
                  </TabsTrigger>
                </>
              ) : currentScanType === "subdomain" ? (
                <TabsTrigger value="subdomains" className="flex-none items-center gap-2">
                  <Captions className="h-4 w-4 shrink-0" />
                  {t.vulnScan.taskManagement.subdomain} ({subdomainTabCount ?? currentTask.subdomainCount ?? currentTask.resultCount ?? 0})
                </TabsTrigger>
              ) : (
                <>
                  <TabsTrigger
                    value="vulnerability"
                    className="flex-none items-center gap-2"
                  >
                    <Bug className="w-4 h-4" />
                    {t.vulnScan.taskDetail.vulnInfo} ({vulnerabilityTabCount ?? (currentTask.vulnCount || 0)})
                  </TabsTrigger>
                </>
              )}
              </TabsList>
            </div>

            {!isMobile && currentScanType === "combination" && activeCombinationResultTabs.length > 0 && (
              <div className="mt-4 min-w-0 border-b border-border/60 pb-3">
                <div className="heph-tabs-scrollbar min-w-0 overflow-x-auto">
                  <TabsList className="h-auto min-w-max flex-nowrap justify-start !gap-1 !rounded-none !border-0 !bg-transparent !p-0">
                    {activeCombinationResultTabs.map(({ value, label, icon: Icon, count }) => (
                      <TabsTrigger
                        key={value}
                        value={value}
                        className="group h-11 md:h-8 flex-none gap-1.5 rounded-md px-2.5 text-xs data-[state=active]:border-border data-[state=active]:bg-muted/60 data-[state=active]:shadow-none"
                      >
                        <Icon className="h-3.5 w-3.5" />
                        <span>{label}</span>
                        <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] leading-none text-muted-foreground group-data-[state=active]:bg-primary/10 group-data-[state=active]:text-primary">
                          {count}
                        </span>
                      </TabsTrigger>
                    ))}
                  </TabsList>
                </div>
              </div>
            )}

            {/* 基础信息标签页 */}
            <TabsContent value="info" className="mt-4 scan-panel-enter">
              <div className="space-y-4">
                {currentTask.errorMsg && (
                  <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/20">
                    <div className="text-sm font-medium text-destructive mb-1">
                      {t.vulnScan.taskDetail.errorInfo}
                    </div>
                    <div className="text-sm text-destructive">
                      {currentTask.errorMsg}
                    </div>
                  </div>
                )}
                {/* 任务基本信息 */}
                <div>
                  <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskDetail.basicInfoTitle}</h3>
                  <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                    <div>
                      <label className="text-xs text-muted-foreground">
                        {t.vulnScan.taskName}
                      </label>
                      <div className="mt-0.5 text-sm font-medium">
                        {currentTask.taskName ||
                          (currentTask.config as any)?.taskName ||
                          t.vulnScan.taskDetail.notSet}
                      </div>
                    </div>
                    <div>
                      <label className="text-xs text-muted-foreground">
                        {t.vulnScan.taskDetail.taskType}
                      </label>
                      <div className="mt-0.5 flex items-center gap-2">
                        <span className="text-sm font-medium">
                          {(() => {
                            const taskType = normalizeScanType(currentTask.scanType);
                            if (taskType === "subdomain") return t.vulnScan.taskManagement.subdomain;
                            if (taskType === "crack") return t.vulnScan.taskManagement.crack;
                            if (taskType === "portscan") return t.vulnScan.taskManagement.portscan;
                            if (taskType === "dirscan") return t.vulnScan.taskManagement.dirscan;
                            if (taskType === "jsscan") return t.vulnScan.taskManagement.jsscan;
                            if (taskType === "combination") return t.vulnScan.taskManagement.combination;
                            return t.vulnScan.taskManagement.webscan;
                          })()}
                        </span>
                        {currentScanType === "webscan" &&
                          (currentTask.config as WebScanConfig).webscanOption && (
                            <Badge variant="secondary" className="text-[10px] px-1.5 py-0 h-5">
                              {(() => {
                                const option =
                                  (currentTask.config as WebScanConfig).webscanOption;
                                const modes = {
                                  1: t.vulnScan.newScan.modes.fingerprint,
                                  2: t.vulnScan.newScan.modes.fullFingerprint,
                                  3: t.vulnScan.newScan.modes.fingerprintVuln,
                                  4: t.vulnScan.newScan.modes.specialScan,
                                };
                                return modes[option as keyof typeof modes] || option;
                              })()}
                            </Badge>
                          )}
                      </div>
                    </div>
                    <div>
                      <label className="text-xs text-muted-foreground">
                        {t.vulnScan.taskDetail.taskStatus}
                      </label>
                      <div className="mt-0.5 text-sm">
                        {getStatusBadge(currentTask.status)}
                      </div>
                    </div>
                    <div>
                      <label className="text-xs text-muted-foreground">
                        {t.vulnScan.taskDetail.scanProgress}
                      </label>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <div className="mt-0.5 text-sm cursor-default">
                            {currentTask.progress}%
                          </div>
                        </TooltipTrigger>
                        <ScanStageTooltipContent events={currentTask.stageEvents} />
                      </Tooltip>
                    </div>
                    <div>
                      <label className="text-xs text-muted-foreground">
                        {t.vulnScan.taskDetail.scanCount}
                      </label>
                      <div className="mt-0.5 text-sm">
                        {t.vulnScan.taskDetail.scanCountFormat.replace('{n}', String(version !== undefined ? version : currentTask.version || 1))}
                        {version !== undefined &&
                          version !== (currentTask.version || 1) && (
                            <span className="text-xs text-muted-foreground ml-2">
                              {t.vulnScan.taskDetail.historyVersion}
                            </span>
                          )}
                      </div>
                    </div>
                    <div>
                      <label className="text-xs text-muted-foreground">
                        {t.vulnScan.createTime}
                      </label>
                      <div className="mt-0.5 text-sm">
                        {formatDate(currentTask.createdAt)}
                      </div>
                    </div>
                    {scheduleSummary && (
                      <div>
                        <label className="text-xs text-muted-foreground">
                          {t.vulnScan.taskDetail.schedule}
                        </label>
                        <div className="mt-0.5 text-sm">
                          {scheduleSummary || t.vulnScan.taskDetail.notSet}
                        </div>
                      </div>
                    )}
                    {(currentScanType === "combination" || currentTask.nextRunAt) && (
                      <div>
                        <label className="text-xs text-muted-foreground">
                          {t.vulnScan.taskDetail.nextRunAt}
                        </label>
                        <div className="mt-0.5 text-sm">
                          {currentTask.nextRunAt ? formatDate(currentTask.nextRunAt) : "-"}
                        </div>
                      </div>
                    )}
                  </div>
                </div>

                {(canEditSchedule || scheduleSummary || scheduleEnabled || scheduleLogs.length > 0) && (
                  <div className="border-t border-border/60 pt-5">
                    <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                      <div>
                        <h3 className="text-sm font-semibold">{t.vulnScan.taskDetail.scheduleTaskTitle}</h3>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">
                          {scheduleStateText}
                        </p>
                      </div>
                      {canEditSchedule && (
                        <div className="flex flex-wrap gap-2">
                          <Button type="button" size="sm" variant="outline" onClick={openScheduleEditor}>
                            <Pencil className="h-4 w-4" />
                            {t.vulnScan.taskDetail.editSchedule}
                          </Button>
                          {scheduleEnabled && (
                            <Button type="button" size="sm" variant="outline" onClick={handleDisableSchedule} disabled={savingSchedule}>
                              <X className="h-4 w-4" />
                              {t.vulnScan.taskDetail.disableSchedule}
                            </Button>
                          )}
                        </div>
                      )}
                    </div>

                    <div className="grid gap-x-8 gap-y-4 md:grid-cols-3">
                      <div>
                        <div className="text-xs text-muted-foreground">{t.vulnScan.taskDetail.schedule}</div>
                        <div className="mt-1 text-sm font-medium">{scheduleSummary || t.vulnScan.taskDetail.notSet}</div>
                      </div>
                      <div>
                        <div className="text-xs text-muted-foreground">{t.vulnScan.taskDetail.nextRunAt}</div>
                        <div className="mt-1 text-sm font-medium">{currentTask.nextRunAt ? formatDate(currentTask.nextRunAt) : "-"}</div>
                      </div>
                      <div>
                        <div className="text-xs text-muted-foreground">{t.vulnScan.taskDetail.recentScheduleRun}</div>
                        <div className="mt-1 flex items-center gap-2 text-sm font-medium">
                          {latestScheduleLog ? (
                            <>
                              <Badge
                                variant={latestScheduleLog.status === "failed" ? "destructive" : "secondary"}
                                className="h-5 px-1.5 text-[10px]"
                              >
                                {logStatusLabels[latestScheduleLog.status]}
                              </Badge>
                              <span>{formatDate(latestScheduleLog.triggeredAt)}</span>
                            </>
                          ) : loadingScheduleLogs ? (
                            <span className="text-muted-foreground">{t.common.loading}</span>
                          ) : (
                            <span className="text-muted-foreground">{t.vulnScan.taskDetail.noScheduleLogs}</span>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="mt-5 border-t border-border/60 pt-4">
                      <div className="mb-2 text-xs font-medium text-muted-foreground">
                        {t.vulnScan.taskDetail.scheduleLogs}
                      </div>
                      {scheduleLogs.length > 0 ? (
                        <div className="space-y-2">
                          {scheduleLogs.map((item) => (
                            <div key={item.id} className="grid gap-2 rounded-md border border-border/50 bg-muted/20 px-3 py-2 text-xs md:grid-cols-[140px_140px_80px_minmax(0,1fr)]">
                              <div>
                                <span className="text-muted-foreground">{t.vulnScan.taskDetail.scheduledAt}: </span>
                                {formatDate(item.scheduledAt)}
                              </div>
                              <div>
                                <span className="text-muted-foreground">{t.vulnScan.taskDetail.triggeredAt}: </span>
                                {formatDate(item.triggeredAt)}
                              </div>
                              <div>
                                <Badge
                                  variant={item.status === "failed" ? "destructive" : "secondary"}
                                  className="h-5 px-1.5 text-[10px]"
                                >
                                  {logStatusLabels[item.status]}
                                </Badge>
                              </div>
                              <div className="truncate text-muted-foreground" title={scheduleLogReason(item) || item.nextRunAt || ""}>
                                {scheduleLogReason(item) || (item.executionVersion ? `${t.vulnScan.taskDetail.scanCount}: ${item.executionVersion}` : `${t.vulnScan.taskDetail.nextRunAt}: ${formatDate(item.nextRunAt)}`)}
                              </div>
                            </div>
                          ))}
                        </div>
                      ) : (
                        <div className="text-xs text-muted-foreground">
                          {loadingScheduleLogs ? t.common.loading : t.vulnScan.taskDetail.noScheduleLogs}
                        </div>
                      )}
                    </div>
                  </div>
                )}

                {/* 目标列表 */}
                <div>
                  {(() => {
                    const targets = currentTask.config.targets || [];
                    const maxDisplay = 20; // 最多显示20个目标
                    const displayTargets = showAllTargets
                      ? targets
                      : targets.slice(0, maxDisplay);
                    const hasMore = targets.length > maxDisplay;

                    return (
                      <>
                        <div className="flex items-center justify-between mb-1.5">
                          <label className="text-sm font-semibold">
                            {t.vulnScan.taskDetail.targetList}
                            {targets.length > 0 && (
                              <span className="text-xs text-muted-foreground font-normal ml-2">
                                {t.vulnScan.taskDetail.targetCount.replace('{n}', String(targets.length))}
                              </span>
                            )}
                          </label>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={async () => {
                              const success = await copyToClipboard(targets.join('\n'))
                              if (success) {
                                toast.success(t.vulnScan.messages.copied)
                              } else {
                                toast.error(t.vulnScan.messages.copyFailed)
                              }
                            }}
                          >
                            <Copy className="w-3 h-3 mr-1" />
                            {t.common.copy}
                          </Button>
                        </div>
                        <div className="border rounded-lg p-2 bg-muted/50">
                          <div className="max-h-[180px] overflow-y-auto">
                            <div className="space-y-0.5">
                              {displayTargets.length === 0 ? (
                                <div className="text-xs text-muted-foreground">-</div>
                              ) : (
                                displayTargets.map((target, index) => (
                                  <div
                                    key={index}
                                    className="text-xs font-mono break-all"
                                  >
                                    {target}
                                  </div>
                                ))
                              )}
                            </div>
                          </div>
                          {hasMore && (
                            <div className="mt-2 pt-2 border-t">
                              <button
                                onClick={() =>
                                  setShowAllTargets(!showAllTargets)
                                }
                                className="text-xs text-primary hover:underline"
                              >
                                {showAllTargets ? (
                                  <>{t.vulnScan.taskDetail.collapse.replace('{n}', String(maxDisplay))}</>
                                ) : (
                                  <>
                                    {t.vulnScan.taskDetail.expand.replace('{n}', String(targets.length - maxDisplay))}
                                  </>
                                )}
                              </button>
                            </div>
                          )}
                        </div>
                      </>
                    );
                  })()}
                </div>

                {/* 组合任务流程概览（不展示具体配置参数） */}
                {currentScanType === "combination" && (
                  <div>
                    <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskDetail.combinationFlow}</h3>
                    {combinationFlowStages.length === 0 ? (
                      <div className="text-sm text-muted-foreground">{t.vulnScan.taskDetail.noFlowData}</div>
                    ) : (
                      <div className="space-y-2">
                        {combinationFlowStages.map((stage) => (
                          <div key={`stage-${stage.stageIndex}`} className="border rounded-lg bg-muted/30 p-3 space-y-2">
                            <div className="flex items-center justify-between">
                              <span className="text-sm font-medium">
                                {t.vulnScan.taskDetail.flowStage.replace("{n}", String(stage.stageOrder + 1))}
                              </span>
                              {stage.nodes.length > 1 && (
                                <Badge variant="outline" className="text-[10px]">
                                  {t.vulnScan.taskDetail.parallelExecution}
                                </Badge>
                              )}
                            </div>
                            <div className="flex flex-wrap gap-2">
                              {stage.nodes.map((node) => {
                                const meta = getCombinationModuleMeta(node.moduleId);
                                const Icon = meta.icon;
                                return (
                                  <Badge key={node.key} variant="outline" className="h-7 px-2 flex items-center gap-1.5">
                                    <Icon className={`h-3.5 w-3.5 shrink-0 ${meta.tone}`} />
                                    <span>{meta.label}</span>
                                  </Badge>
                                );
                              })}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                {/* 网站扫描特有配置 */}
                {currentScanType === "webscan" && (() => {
                  const webConfig = currentTask.config as WebScanConfig;
                  const interactshURL = webConfig.interactshURL || webConfig.interactshUrl || "";
                  const interactshToken = (webConfig.interactshToken || "").trim();
                  const oobEnabled =
                    webConfig.oobEnabled === true ||
                    Boolean(interactshURL) ||
                    Boolean(interactshToken);
                  return (
                    <div>
                      <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskDetail.webscanConfig}</h3>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        <div className="col-span-full">
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.customHeadersLabel}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {webConfig.customHeaders || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.specifyFingerprint}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.customTags?.join(", ") ||
                              t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.specifyVuln}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.customTemplate?.join(", ") ||
                              t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.rootPathScan}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.rootPathScan ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.skipNucleiWithoutTags}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.skipNucleiWithoutTags
                              ? t.vulnScan.taskDetail.yes
                              : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.skipNucleiWithWAF}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.skipNucleiWithWAF !== false
                              ? t.vulnScan.taskDetail.yes
                              : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.screenshot}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.screenshot ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.activeTimeoutLimit}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.activeTimeoutLimit ||
                              t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.disableMaxHostErr}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.disableMaxHostErr
                              ? t.vulnScan.taskDetail.yes
                              : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.threads}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.threads || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.nucleiThreads}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {webConfig.nucleiThreads || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.oobEnabled}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {oobEnabled ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div className="sm:col-span-2">
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.interactshURL}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {interactshURL || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.interactshToken}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {interactshToken ? t.vulnScan.taskDetail.configured : t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })()}

                {/* 子域名收集特有配置 */}
                {currentScanType === "subdomain" && (() => {
                  const subdomainConfig = currentTask.config as SubdomainScanConfig;
                  return (
                    <div>
                      <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskManagement.subdomain}</h3>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.subdomain.configSheet.threadCount}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {subdomainConfig.thread || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.subdomain.configSheet.resolveTimeout}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {subdomainConfig.timeout ? `${subdomainConfig.timeout}s` : t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div className="col-span-full">
                          <label className="text-xs text-muted-foreground">
                            {t.subdomain.configSheet.dnsServers}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {subdomainConfig.dnsServers?.length
                              ? subdomainConfig.dnsServers.join(", ")
                              : t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div className="col-span-full">
                          <label className="text-xs text-muted-foreground">
                            {t.subdomain.configSheet.selectEngines}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {subdomainConfig.appendEngines?.length
                              ? subdomainConfig.appendEngines.join(", ")
                              : t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })()}

                {/* 端口扫描特有配置 */}
                {currentScanType === "portscan" && (() => {
                  const portConfig = currentTask.config as PortScanConfig;
                  const rawDiscoveryMode = String(
                    portConfig.tcpDiscoveryMode || portConfig.portDiscoveryMode || ""
                  ).toLowerCase();
                  const discoveryMode = rawDiscoveryMode === "connect" ? "connect" : "syn";
                  const discoveryModeLabel =
                    discoveryMode === "connect"
                      ? t.vulnScan.taskDetail.connectMode
                      : rawDiscoveryMode
                        ? t.vulnScan.taskDetail.synMode
                        : t.vulnScan.taskDetail.synModeDefault;
                  const synRate = Number(
                    portConfig.synRate ?? portConfig.portDiscoveryRate ?? portConfig.discoveryRate
                  );
                  return (
                    <div>
                      <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskDetail.portScanConfig}</h3>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        <div className="sm:col-span-2">
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.portRange}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {portConfig.portRange || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.timeout}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {portConfig.timeout ? `${portConfig.timeout}s` : t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.aliveDetection}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {portConfig.aliveDetection ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">{t.vulnScan.newScan.subnetDiscovery.title}</label>
                          <div className="mt-0.5 text-sm">{portConfig.subnetDiscoveryEnabled ? t.vulnScan.newScan.subnetDiscovery[normalizeTCPHeuristicConfig(portConfig).subnetDiscoveryMode] : t.vulnScan.taskDetail.no}</div>
                        </div>
                        {portConfig.subnetDiscoveryEnabled && <>
                          <div>
                            <label className="text-xs text-muted-foreground">{t.vulnScan.newScan.subnetDiscovery.suffixes}</label>
                            <div className="mt-0.5 break-all font-mono text-xs">{normalizeTCPHeuristicConfig(portConfig).subnetDiscoveryHostSuffixes}</div>
                          </div>
                          <div>
                            <label className="text-xs text-muted-foreground">{t.vulnScan.newScan.subnetDiscovery.randomSamples}</label>
                            <div className="mt-0.5 text-sm">{normalizeTCPHeuristicConfig(portConfig).subnetDiscoveryRandomSamples}</div>
                          </div>
                        </>}
                        <div>
                          <label className="text-xs text-muted-foreground">{t.vulnScan.newScan.tcpHeuristic.title}</label>
                          <div className="mt-0.5 text-sm">{(portConfig.tcpHeuristicEnabled || portConfig.subnetDiscoveryEnabled) ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}</div>
                        </div>
                        {(portConfig.tcpHeuristicEnabled || portConfig.subnetDiscoveryEnabled) && (
                          <>
                            <div>
                              <label className="text-xs text-muted-foreground">{t.vulnScan.newScan.tcpHeuristic.ports}</label>
                              <div className="mt-0.5 break-all font-mono text-xs">{normalizeTCPHeuristicConfig(portConfig).tcpHeuristicPorts}</div>
                            </div>
                            <div>
                              <label className="text-xs text-muted-foreground">{t.vulnScan.newScan.tcpHeuristic.timeout}</label>
                              <div className="mt-0.5 text-sm">{normalizeTCPHeuristicConfig(portConfig).tcpHeuristicTimeoutMs / 1000}s</div>
                            </div>
                          </>
                        )}
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.tcpDiscoveryMode}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {discoveryModeLabel}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.threads}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {portConfig.threads || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        {discoveryMode === "syn" && (
                          <div>
                            <label className="text-xs text-muted-foreground">
                              {t.vulnScan.taskDetail.synRate}
                            </label>
                            <div className="mt-0.5 text-sm">
                              {Number.isFinite(synRate) && synRate > 0
                                ? synRate
                                : t.vulnScan.taskDetail.defaultValue}
                            </div>
                          </div>
                        )}
                      </div>
                    </div>
                  );
                })()}

                {/* JS 扫描特有配置 */}
                {currentScanType === "jsscan" && (() => {
                  const jsConfig = currentTask.config as JsScanConfig;
                  const vulnDetectionEnabled = jsConfig.vulnDetection?.enabled === true;
                  return (
                    <div>
                      <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskDetail.jsScanConfig}</h3>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.aiAssistedDetection}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {jsConfig.openai?.enabled ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.vulnerabilityDetection}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {vulnDetectionEnabled ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })()}

                {/* 密码暴破特有配置 */}
                {currentScanType === "crack" && (() => {
                  const crackConfig = currentTask.config as CrackScanConfig;
                  return (
                    <div>
                      <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskDetail.crackConfig}</h3>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.useDefaultDict}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {crackConfig.defaultDict ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div className="col-span-full">
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.userDict}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {crackConfig.username || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                        <div className="col-span-full">
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.passwordDict}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {crackConfig.password || t.vulnScan.taskDetail.notSet}
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })()}

                {/* 目录扫描特有配置 */}
                {currentScanType === "dirscan" && (() => {
                  const dirConfig = currentTask.config as any;
                  return (
                    <div>
                      <h3 className="text-sm font-semibold mb-2">{t.vulnScan.taskDetail.dirScanConfig}</h3>
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.requestMethod}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {dirConfig.method || 'GET'}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.concurrentThreads}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {dirConfig.workers || 25}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.timeout}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {dirConfig.timeout ? `${dirConfig.timeout}s` : '-'}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.followRedirect}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {dirConfig.redirect ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.backupFileScan}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {dirConfig.backupscan ? t.vulnScan.taskDetail.yes : t.vulnScan.taskDetail.no}
                          </div>
                        </div>
                        <div>
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.recursionDepth}
                          </label>
                          <div className="mt-0.5 text-sm">
                            {dirConfig.recursive || 0}
                          </div>
                        </div>
                        <div className="col-span-full">
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.extensions}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {dirConfig.exts?.length ? dirConfig.exts.join(', ') : '-'}
                          </div>
                        </div>
                        <div className="col-span-full">
                          <label className="text-xs text-muted-foreground">
                            {t.vulnScan.taskDetail.filterStatusCode}
                          </label>
                          <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                            {dirConfig.statusCodeExclude?.length ? dirConfig.statusCodeExclude.join(', ') : '-'}
                          </div>
                        </div>
                        {dirConfig.dictPaths?.length > 0 && (
                          <div className="col-span-full">
                            <label className="text-xs text-muted-foreground">
                              {t.vulnScan.taskDetail.dictionaryPath}
                            </label>
                            <div className="mt-0.5 text-xs font-mono break-all bg-muted/50 p-1.5 rounded">
                              {dirConfig.dictPaths.join(', ')}
                            </div>
                          </div>
                        )}
                        {dirConfig.paths?.length > 0 && (
                          <div className="col-span-full">
                            <label className="text-xs text-muted-foreground">
                              {t.vulnScan.taskDetail.customPaths}
                            </label>
                            <div className="mt-0.5 text-xs font-mono whitespace-pre-wrap break-all bg-muted/50 p-1.5 rounded max-h-32 overflow-y-auto">
                              {dirConfig.paths.join('\n')}
                            </div>
                          </div>
                        )}
                      </div>
                    </div>
                  );
                })()}
              </div>
            </TabsContent>

            {currentScanType === "webscan" ? (
              <>
                <TabsContent value="fingerprint" className="mt-3 md:mt-6 scan-panel-enter">
                  {effectiveFingerprintLayout === "table" ? (
                    <FingerprintTable
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      currentLayout={effectiveFingerprintLayout}
                      onLayoutChange={(layout) => setFingerprintLayout(layout)}
                      onTotalChange={setFingerprintTabCount}
                    />
                  ) : (
                    <FingerprintCardView
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      currentLayout={effectiveFingerprintLayout}
                      onLayoutChange={(layout) => setFingerprintLayout(layout)}
                      onTotalChange={setFingerprintTabCount}
                    />
                  )}
                </TabsContent>
                <TabsContent value="vulnerability" className="mt-3 md:mt-6 scan-panel-enter">
                  <VulnerabilityView
                    taskId={currentTaskId || undefined}
                    version={effectiveVersion}
                    taskName={currentTask.taskName || ''}
                    onTotalChange={setVulnerabilityTabCount}
                    onViewDetailExternal={openWebFinding}
                    onVulnerabilityDeleted={() => {
                      // No need to reload - VulnerabilityView will refresh itself
                    }}
                  />
                </TabsContent>
              </>
            ) : currentScanType === "portscan" ? (
              <>
                <TabsContent value="port" className="mt-3 md:mt-6 scan-panel-enter">
                  <PortScanResultTable
                    taskId={currentTaskId || undefined}
                    version={effectiveVersion}
                    pollWhileRunning={currentTask.status === "running"}
                    onTotalChange={setPortTabCount}
                  />
                </TabsContent>
              </>
            ) : currentScanType === "jsscan" ? (
              <>
                <TabsContent value="vulnerabilities" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "vulnerabilities" && (
                    <JsScanVulnerabilityTable
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanVulnCount}
                      onViewDetail={openJsFinding}
                    />
                  )}
                </TabsContent>
                <TabsContent value="assets" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "assets" && (
                    <JsScanAssetTable
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanAssetCount}
                    />
                  )}
                </TabsContent>
                <TabsContent value="sitetree" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "sitetree" && (
                    <JsScanSiteTree
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanSiteTreeCount}
                    />
                  )}
                </TabsContent>
                <TabsContent value="interfaces" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "interfaces" && (
                    <JsScanInterfaceTable
                      key={`${currentTaskId}-${effectiveVersion}`}
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanInterfaceCount}
                    />
                  )}
                </TabsContent>
              </>
            ) : currentScanType === "dirscan" ? (
              <>
                <TabsContent value="dirresults" className="mt-3 md:mt-6 scan-panel-enter">
                  <DirScanResultTable
                    taskId={currentTaskId || ""}
                    version={effectiveVersion}
                    config={currentTask.config as any}
                  />
                </TabsContent>
              </>
            ) : currentScanType === "combination" ? (
              <>
                <TabsContent value="subdomains" className="mt-3 md:mt-6 scan-panel-enter">
                  <SubdomainResultsPanel
                    taskId={currentTaskId || undefined}
                    totalHint={subdomainTabCount ?? currentTask.subdomainCount ?? 0}
                    onTotalChange={setSubdomainTabCount}
                  />
                </TabsContent>
                <TabsContent value="port" className="mt-3 md:mt-6 scan-panel-enter">
                  <PortScanResultTable
                    taskId={currentTaskId || undefined}
                    version={effectiveVersion}
                    pollWhileRunning={currentTask.status === "running"}
                    onTotalChange={setPortTabCount}
                  />
                </TabsContent>
                <TabsContent value="fingerprint" className="mt-3 md:mt-6 scan-panel-enter">
                  {effectiveFingerprintLayout === "table" ? (
                    <FingerprintTable
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      currentLayout={effectiveFingerprintLayout}
                      onLayoutChange={(layout) => setFingerprintLayout(layout)}
                      onTotalChange={setFingerprintTabCount}
                    />
                  ) : (
                    <FingerprintCardView
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      currentLayout={effectiveFingerprintLayout}
                      onLayoutChange={(layout) => setFingerprintLayout(layout)}
                      onTotalChange={setFingerprintTabCount}
                    />
                  )}
                </TabsContent>
                <TabsContent value="vulnerability" className="mt-3 md:mt-6 scan-panel-enter">
                  <VulnerabilityView
                    taskId={currentTaskId || undefined}
                    version={effectiveVersion}
                    taskName={currentTask.taskName || ''}
                    onTotalChange={setVulnerabilityTabCount}
                    onViewDetailExternal={openWebFinding}
                    onVulnerabilityDeleted={() => {
                      // No need to reload - VulnerabilityView will refresh itself
                    }}
                  />
                </TabsContent>
                <TabsContent value="dirresults" className="mt-3 md:mt-6 scan-panel-enter">
                  <DirScanResultTable
                    taskId={currentTaskId || ""}
                    version={effectiveVersion}
                    config={currentTask.config as any}
                  />
                </TabsContent>
                <TabsContent value="vulnerabilities" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "vulnerabilities" && (
                    <JsScanVulnerabilityTable
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanVulnCount}
                      onViewDetail={openJsFinding}
                    />
                  )}
                </TabsContent>
                <TabsContent value="assets" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "assets" && (
                    <JsScanAssetTable
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanAssetCount}
                    />
                  )}
                </TabsContent>
                <TabsContent value="sitetree" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "sitetree" && (
                    <JsScanSiteTree
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanSiteTreeCount}
                    />
                  )}
                </TabsContent>
                <TabsContent value="interfaces" className="mt-3 md:mt-6 scan-panel-enter">
                  {activeTab === "interfaces" && (
                    <JsScanInterfaceTable
                      key={`${currentTaskId}-${effectiveVersion}`}
                      taskId={currentTaskId || undefined}
                      version={effectiveVersion}
                      onTotalChange={setJsScanInterfaceCount}
                    />
                  )}
                </TabsContent>
              </>
            ) : currentScanType === "subdomain" ? (
              <TabsContent value="subdomains" className="mt-3 md:mt-6 scan-panel-enter">
                <SubdomainResultsPanel
                  taskId={currentTaskId || undefined}
                  totalHint={subdomainTabCount ?? currentTask.subdomainCount ?? currentTask.resultCount ?? 0}
                  onTotalChange={setSubdomainTabCount}
                />
              </TabsContent>
            ) : (
              <>
                <TabsContent value="port" className="mt-3 md:mt-6 scan-panel-enter">
                  <PortScanResultTable
                    taskId={currentTaskId || undefined}
                    version={effectiveVersion}
                    pollWhileRunning={currentTask.status === "running"}
                    onTotalChange={setPortTabCount}
                  />
                </TabsContent>
                <TabsContent value="fingerprint" className="mt-3 md:mt-6 scan-panel-enter">
                  {effectiveFingerprintLayout === "table" ? (
                    <FingerprintTable
                      taskId={currentTaskId || undefined}
                      version={version}
                      currentLayout={effectiveFingerprintLayout}
                      onLayoutChange={(layout) => setFingerprintLayout(layout)}
                      onTotalChange={setFingerprintTabCount}
                    />
                  ) : (
                    <FingerprintCardView
                      taskId={currentTaskId || undefined}
                      version={version}
                      currentLayout={effectiveFingerprintLayout}
                      onLayoutChange={(layout) => setFingerprintLayout(layout)}
                      onTotalChange={setFingerprintTabCount}
                    />
                  )}
                </TabsContent>
                <TabsContent value="vulnerability" className="mt-3 md:mt-6 scan-panel-enter">
                  <VulnerabilityView
                    taskId={currentTaskId || undefined}
                    version={version}
                    taskName={currentTask.taskName || ''}
                    onTotalChange={setVulnerabilityTabCount}
                    onViewDetailExternal={openWebFinding}
                    onVulnerabilityDeleted={() => {
                      // No need to reload - VulnerabilityView will refresh itself
                    }}
                  />
                </TabsContent>
              </>
            )}
          </Tabs>
          </DetailCategoryContext.Provider>
        </Card>
        <Dialog open={scheduleDialogOpen} onOpenChange={setScheduleDialogOpen}>
          <DialogContent className="max-h-[calc(100vh-4rem)] overflow-y-auto sm:max-w-[920px] xl:max-w-[1040px]">
            <DialogHeader>
              <DialogTitle>{t.vulnScan.taskDetail.editSchedule}</DialogTitle>
              <DialogDescription>{t.vulnScan.taskDetail.editScheduleDesc}</DialogDescription>
            </DialogHeader>
            <TaskScheduleSettings
              value={scheduleDraft}
              onChange={setScheduleDraft}
              disabled={savingSchedule}
            />
            <DialogFooter className="sticky bottom-0 -mx-6 -mb-6 border-t border-border/60 bg-background/95 px-6 py-4 backdrop-blur">
              <Button
                type="button"
                variant="outline"
                onClick={() => setScheduleDialogOpen(false)}
                disabled={savingSchedule}
              >
                {t.common.cancel}
              </Button>
              <Button type="button" onClick={handleSaveSchedule} disabled={savingSchedule}>
                {savingSchedule ? t.common.updating : t.vulnScan.taskDetail.saveSchedule}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

      </div>
    </div>
  );
}
