"use client";

import { useState, useEffect, useMemo } from "react";
import Link from "next/link";
import { findingDetailHref } from "@/lib/finding-detail";
import { usePathname, useRouter } from "next/navigation";
import { cn } from "@/lib/utils";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  ArrowDownUp,
  Globe,
  Shield,
  Bot,
  CheckCircle2,
  Activity,
  Cpu,
  Target,
  Clock,
  Server,
  Scan,
  Bug,
  TrendingUp,
  XCircle,
  Layers,
  Monitor,
  HardDrive,
  Plug,
  Fingerprint,
  FileCode,
  FolderSearch,
  Captions,
  Ban,
  BellOff,
  Info,
  ListChecks,
  Siren,
  ShieldAlert,
  Flame,
  ShieldCheck,
  TrendingDown,
  Gauge,
} from "lucide-react";
import { fetchWithAuth } from "@/lib/api";
import { getWebSocketBaseURL } from "@/lib/env";
import { useI18n } from "@/lib/i18n";
import {
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RechartsTooltip,
  ResponsiveContainer,
  AreaChart,
  Area,
  BarChart,
  Bar,
} from "recharts";

type SystemStatsSnapshot = {
  cpu: number;
  mem: number;
  memUsedGB: number;
  memTotalGB: number;
  disk: number;
  networkRxMbps: number;
  networkTxMbps: number;
  uptimeSec: number;
  diskUsedGB: number;
  diskFreeGB: number;
  diskTotalGB: number;
  platform: string;
  arch: string;
  os: string;
  hostname: string;
};

type SystemMetricKey = "cpu" | "mem" | "disk" | "network";

type SystemMetricHistoryPoint = {
  index: number;
  cpu: number;
  mem: number;
  disk: number;
  network: number;
};

type SystemStatsHistorySample = {
  sampledAt: string;
  cpu: number;
  mem: number;
  memUsedGB: number;
  memTotalGB: number;
  disk: number;
  networkRxMbps: number;
  networkTxMbps: number;
  diskUsedGB: number;
  diskFreeGB: number;
  diskTotalGB: number;
};

type SystemStatsHistoryResponse = {
  success?: boolean;
  data?: {
    latest?: Partial<SystemStatsSnapshot>;
    samples?: SystemStatsHistorySample[];
  };
};

type TokenUsagePoint = {
  date: string;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
};

type DashboardTokenUsage = {
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  trend: TokenUsagePoint[];
};

// 新的统一任务接口
interface DashboardTask {
  id: string;
  taskName: string;
  taskType: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

// The API may temporarily contain the same logical task from both the
// unified and legacy task sources. Keep the first item because the endpoint
// returns recent tasks in descending creation order.
function deduplicateDashboardTasks(tasks: DashboardTask[]): DashboardTask[] {
  const seen = new Set<string>();
  return tasks.filter((task) => {
    const key = `${task.taskType}\u0000${task.id}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

interface DashboardStats {
  tasks: {
    total: number;
    byType: {
      webscan: number;
      portscan: number;
      subdomain: number;
      dirsearch: number;
      dirscan: number;
      jsscan: number;
      crack: number;
      combination: number;
    };
    running: number;
    completed: number;
    failed: number;
    canceled: number;
  };
  vulnerabilities: {
    total: number;
    critical: number;
    high: number;
    medium: number;
    low: number;
    info: number;
    disposal: {
      open: number;
      inProgress: number;
      fixed: number;
      falsePositive: number;
      ignored: number;
    };
  };
  assets: {
    ips: number;
    ports: number;
    subdomains: number;
    websites: number;
  };
  efficiency: {
    avgDuration: number;
    successRate: number;
    scansToday: number;
  };
}

const STATUS_COLORS = {
  completed: {
    bg: "bg-tone-green-soft",
    text: "text-tone-green",
    icon: CheckCircle2,
  },
  running: { bg: "bg-primary/20", text: "text-primary", icon: Activity },
  failed: { bg: "bg-tone-red-soft", text: "text-tone-red", icon: XCircle },
  pending: { bg: "bg-tone-amber-soft", text: "text-tone-amber", icon: Clock },
  cancelled: {
    bg: "dashboard-status-muted-surface",
    text: "dashboard-status-muted-text",
    icon: XCircle,
  },
};

interface RecentVuln {
  sourceType?: string;
  id?: string;
  taskId?: string;
  vulnName?: string;
  templateId?: string;
  target: string;
  severity: "critical" | "high" | "medium" | "low" | "info";
}

const normalizeDashboardTaskType = (type?: string) => {
  const value = String(type || "")
    .toLowerCase()
    .trim();
  if (value === "dirsearch" || value === "dirscan") return "dirscan";
  if (
    value === "combination_task" ||
    value === "combination-task" ||
    value === "combination"
  )
    return "combination";
  if (
    value === "webscan" ||
    value === "portscan" ||
    value === "crack" ||
    value === "subdomain" ||
    value === "jsscan"
  )
    return value;
  return "unknown";
};

type DashboardTaskType = Exclude<
  ReturnType<typeof normalizeDashboardTaskType>,
  "unknown"
>;

const formatCompactCount = (value: number) => {
  const absValue = Math.abs(value);

  if (absValue < 1000) {
    return String(value);
  }

  const units = [
    { threshold: 1_000_000_000, suffix: "B" },
    { threshold: 1_000_000, suffix: "M" },
    { threshold: 1_000, suffix: "k" },
  ];

  const unit = units.find(({ threshold }) => absValue >= threshold);
  if (!unit) {
    return String(value);
  }

  const scaledValue = value / unit.threshold;
  const digits = scaledValue >= 100 ? 0 : 1;

  return `${parseFloat(scaledValue.toFixed(digits))}${unit.suffix}`;
};

type TokenUsageChartPoint = TokenUsagePoint & { historyTokens: number };

function DashboardDateRange({
  value,
  onChange,
  dayLabel,
}: {
  value: number;
  onChange: (days: number) => void;
  dayLabel: string;
}) {
  return (
    <div className="flex items-center gap-0.5 rounded-md border border-border/70 bg-muted/40 p-0.5">
      {[7, 14, 30].map((days) => (
        <button
          key={days}
          type="button"
          aria-pressed={value === days}
          onClick={() => onChange(days)}
          className={cn(
            "rounded px-2 py-0.5 text-xs transition-colors",
            value === days
              ? "bg-muted text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {days}{dayLabel}
        </button>
      ))}
    </div>
  );
}

function TokenUsageTooltip({
  active,
  payload,
  label,
  messages,
}: {
  active?: boolean;
  payload?: Array<{ payload?: TokenUsageChartPoint }>;
  label?: string | number;
  messages: {
    total: string;
    input: string;
    output: string;
    history: string;
    cache: string;
  };
}) {
  const point = payload?.[0]?.payload;
  if (!active || !point) {
    return null;
  }
  const rows: Array<[string, number]> = [
    [messages.total, point.totalTokens],
    [messages.input, point.inputTokens],
    [messages.output, point.outputTokens],
  ];
  if (point.historyTokens > 0) {
    rows.push([messages.history, point.historyTokens]);
  }
  if (point.cacheReadTokens > 0) {
    rows.push([messages.cache, point.cacheReadTokens]);
  }
  return (
    <div className="min-w-[188px] rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lg">
      <p className="mb-1.5 font-semibold text-foreground">
        {String(label ?? point.date)}
      </p>
      <div className="space-y-1">
        {rows.map(([name, value]) => (
          <div key={name} className="flex items-center justify-between gap-6">
            <span className="text-muted-foreground">{name}</span>
            <span className="font-medium tabular-nums text-foreground">
              {value.toLocaleString("en-US")}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

const formatStorageGB = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) {
    return "--";
  }

  if (value >= 100) {
    return `${value.toFixed(0)} GB`;
  }

  return `${value.toFixed(1)} GB`;
};

const roundMetricValue = (value: number) =>
  Math.round((Number(value) || 0) * 10) / 10;

const normalizeSystemStatsSnapshot = (
  data?: Partial<SystemStatsSnapshot> | null,
): SystemStatsSnapshot => ({
  cpu: roundMetricValue(data?.cpu ?? 0),
  mem: roundMetricValue(data?.mem ?? 0),
  memUsedGB: roundMetricValue(data?.memUsedGB ?? 0),
  memTotalGB: roundMetricValue(data?.memTotalGB ?? 0),
  disk: roundMetricValue(data?.disk ?? 0),
  networkRxMbps: roundMetricValue(data?.networkRxMbps ?? 0),
  networkTxMbps: roundMetricValue(data?.networkTxMbps ?? 0),
  uptimeSec: Math.max(0, Math.round(Number(data?.uptimeSec ?? 0))),
  diskUsedGB: roundMetricValue(data?.diskUsedGB ?? 0),
  diskFreeGB: roundMetricValue(data?.diskFreeGB ?? 0),
  diskTotalGB: roundMetricValue(data?.diskTotalGB ?? 0),
  platform: data?.platform || "",
  arch: data?.arch || "",
  os: data?.os || "",
  hostname: data?.hostname || "",
});

const normalizeSystemHistoryPoint = (
  sample: Partial<SystemStatsHistorySample>,
  index: number,
): SystemMetricHistoryPoint => ({
  index,
  cpu: roundMetricValue(sample.cpu ?? 0),
  mem: roundMetricValue(sample.mem ?? 0),
  disk: roundMetricValue(sample.disk ?? 0),
  network: roundMetricValue(
    (sample.networkRxMbps ?? 0) + (sample.networkTxMbps ?? 0),
  ),
});

const SYSTEM_HISTORY_LIMIT = 100;
const formatThroughput = (value: number) =>
  `${roundMetricValue(value).toFixed(1)} Mbps`;

const createSystemHistory = (
  stats?: Pick<
    SystemStatsSnapshot,
    "cpu" | "mem" | "disk" | "networkRxMbps" | "networkTxMbps"
  >,
): SystemMetricHistoryPoint[] => {
  const snapshot = {
    cpu: stats?.cpu ?? 0,
    mem: stats?.mem ?? 0,
    disk: stats?.disk ?? 0,
    network: (stats?.networkRxMbps ?? 0) + (stats?.networkTxMbps ?? 0),
  };

  return Array.from({ length: SYSTEM_HISTORY_LIMIT }, (_, index) => {
    const progress = (index + 1) / SYSTEM_HISTORY_LIMIT;
    const wave = Math.sin(progress * Math.PI * 1.6) * (1 - progress) * 6;

    return {
      index,
      cpu: Math.max(0, Math.min(100, snapshot.cpu - 3 + wave)),
      mem: Math.max(0, Math.min(100, snapshot.mem - 4 + wave * 0.7)),
      disk: Math.max(0, Math.min(100, snapshot.disk - 2 + wave * 0.45)),
      network: Math.max(0, snapshot.network + wave * 0.25),
    };
  });
};

const isEmptySystemHistory = (history: SystemMetricHistoryPoint[]) =>
  history.every(
    (item) =>
      item.cpu === 0 && item.mem === 0 && item.disk === 0 && item.network === 0,
  );

const getSparklineDomain = (values: number[]) => {
  if (values.length === 0) {
    return [0, 100] as const;
  }

  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min;

  if (range < 6) {
    const midpoint = (max + min) / 2;
    return [Math.max(0, midpoint - 6), Math.min(100, midpoint + 6)] as const;
  }

  return [
    Math.max(0, min - Math.max(2, range * 0.2)),
    Math.min(100, max + Math.max(2, range * 0.2)),
  ] as const;
};

const EMPTY_SYSTEM_STATS: SystemStatsSnapshot = {
  cpu: 0,
  mem: 0,
  memUsedGB: 0,
  memTotalGB: 0,
  disk: 0,
  networkRxMbps: 0,
  networkTxMbps: 0,
  uptimeSec: 0,
  diskUsedGB: 0,
  diskFreeGB: 0,
  diskTotalGB: 0,
  platform: "",
  arch: "",
  os: "",
  hostname: "",
};

const dashboardSystemMonitorCache: {
  stats: SystemStatsSnapshot;
  history: SystemMetricHistoryPoint[];
  wsConnected: boolean;
} = {
  stats: EMPTY_SYSTEM_STATS,
  history: createSystemHistory(),
  wsConnected: false,
};

export default function DashboardPage() {
  const router = useRouter();
  const { t } = useI18n();
  const [recentTasks, setRecentTasks] = useState<DashboardTask[]>([]);
  const [recentVulns, setRecentVulns] = useState<RecentVuln[]>([]);
  const pathname = usePathname();
  const [stats, setStats] = useState<DashboardStats>({
    tasks: {
      total: 0,
      byType: {
        webscan: 0,
        portscan: 0,
        subdomain: 0,
        dirsearch: 0,
        dirscan: 0,
        jsscan: 0,
        crack: 0,
        combination: 0,
      },
      running: 0,
      completed: 0,
      failed: 0,
      canceled: 0,
    },
    vulnerabilities: {
      total: 0,
      critical: 0,
      high: 0,
      medium: 0,
      low: 0,
      info: 0,
      disposal: {
        open: 0,
        inProgress: 0,
        fixed: 0,
        falsePositive: 0,
        ignored: 0,
      },
    },
    assets: { ips: 0, ports: 0, subdomains: 0, websites: 0 },
    efficiency: { avgDuration: 0, successRate: 0, scansToday: 0 },
  });
  const [trendData, setTrendData] = useState<
    {
      name: string;
      critical: number;
      high: number;
      medium: number;
      low: number;
    }[]
  >([]);
  const [vulnTrendDays, setVulnTrendDays] = useState(30);
  const [trendLoading, setTrendLoading] = useState(false);
  const [tokenUsage, setTokenUsage] = useState<DashboardTokenUsage>({
    totalTokens: 0,
    inputTokens: 0,
    outputTokens: 0,
    cacheReadTokens: 0,
    trend: [],
  });
  const [tokenUsageDays, setTokenUsageDays] = useState(30);
  const [tokenUsageLoading, setTokenUsageLoading] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  // 系统性能监控状态
  const [systemStats, setSystemStats] = useState<SystemStatsSnapshot>(
    () => dashboardSystemMonitorCache.stats,
  );
  const [systemMetricHistory, setSystemMetricHistory] = useState<
    SystemMetricHistoryPoint[]
  >(() => dashboardSystemMonitorCache.history);
  const [wsConnected, setWsConnected] = useState(
    () => dashboardSystemMonitorCache.wsConnected,
  );

  // 加载趋势数据（独立加载，支持切换天数）
  useEffect(() => {
    const loadTrendData = async () => {
      setTrendLoading(true);
      try {
        const trendRes = await fetchWithAuth(
          `/api/v1/webscan/trend?days=${vulnTrendDays}`,
        ).catch(() => null);
        if (trendRes?.ok) {
          const data = await trendRes.json();
          if (data.success && data.data?.trend) {
            const mapped = data.data.trend.map(
              (item: {
                date: string;
                critical: number;
                high: number;
                medium: number;
                low: number;
              }) => {
                const date = new Date(item.date);
                return {
                  name: `${date.getMonth() + 1}/${date.getDate()}`,
                  critical: item.critical || 0,
                  high: item.high || 0,
                  medium: item.medium || 0,
                  low: item.low || 0,
                };
              },
            );
            setTrendData(mapped);
          } else {
            setTrendData([]);
          }
        } else {
          setTrendData([]);
        }
      } catch (error) {
        console.error("Failed to load trend data:", error);
        setTrendData([]);
      } finally {
        setTrendLoading(false);
      }
    };
    loadTrendData();
  }, [vulnTrendDays]);

  useEffect(() => {
    if (pathname !== "/dashboard") return;
    let active = true;
    setTokenUsageLoading(true);
    fetchWithAuth(`/api/v1/dashboard/token-usage?days=${tokenUsageDays}`)
      .then((res) => (res.ok ? res.json() : null))
      .then((data) => {
        if (!active || !data?.success || !data.data) return;
        const byDate = new Map<string, TokenUsagePoint>(
          (Array.isArray(data.data.trend) ? data.data.trend : []).map(
            (point: TokenUsagePoint) => [point.date, point],
          ),
        );
        // The API groups usage by UTC date, so fill missing days with UTC keys.
        // Using local midnight here would drop the newest day for UTC+ zones.
        const today = new Date();
        const completeTrend = Array.from({ length: tokenUsageDays }, (_, index) => {
          const date = new Date(
            Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate()),
          );
          date.setUTCDate(date.getUTCDate() - tokenUsageDays + 1 + index);
          const key = date.toISOString().slice(0, 10);
          return byDate.get(key) || {
            date: key,
            totalTokens: 0,
            inputTokens: 0,
            outputTokens: 0,
            cacheReadTokens: 0,
          };
        });
        setTokenUsage({
          totalTokens: Number(data.data.totalTokens || 0),
          inputTokens: Number(data.data.inputTokens || 0),
          outputTokens: Number(data.data.outputTokens || 0),
          cacheReadTokens: Number(data.data.cacheReadTokens || 0),
          trend: completeTrend,
        });
      })
      .catch(() => undefined)
      .finally(() => active && setTokenUsageLoading(false));
    return () => {
      active = false;
    };
  }, [pathname, tokenUsageDays]);

  const tokenUsageSummary = useMemo(() => {
    const points = tokenUsage.trend;
    const total = points.reduce(
      (sum, point) => sum + Number(point.totalTokens || 0),
      0,
    );
    const input = points.reduce(
      (sum, point) => sum + Number(point.inputTokens || 0),
      0,
    );
    const output = points.reduce(
      (sum, point) => sum + Number(point.outputTokens || 0),
      0,
    );
    const cache = points.reduce(
      (sum, point) => sum + Number(point.cacheReadTokens || 0),
      0,
    );
    // Historical rows may only carry a total, so keep the unmatched remainder
    // visible instead of pretending input + output covers the whole range.
    const detailed = input + output;
    const history = Math.max(total - detailed, 0);
    const denominator = Math.max(total, detailed, 1);
    const chartData: TokenUsageChartPoint[] = points.map((point) => {
      const pointTotal = Number(point.totalTokens || 0);
      const pointDetailed =
        Number(point.inputTokens || 0) + Number(point.outputTokens || 0);
      return {
        ...point,
        historyTokens: Math.max(pointTotal - pointDetailed, 0),
      };
    });
    return {
      total,
      input,
      output,
      cache,
      history,
      chartData,
      hasUsage: total > 0 || detailed > 0,
      inputShare: input / denominator,
      outputShare: output / denominator,
      historyShare: history / denominator,
      cacheInputShare: input > 0 ? cache / input : 0,
    };
  }, [tokenUsage.trend]);

  // WebSocket 连接获取系统性能数据
  useEffect(() => {
    let isActive = true;
    let ws: WebSocket | null = null;

    const connectWebSocket = () => {
      const token = localStorage.getItem("auth_token");
      if (!token || !isActive) return;

      const wsBaseUrl = getWebSocketBaseURL();
      const wsUrl = `${wsBaseUrl}/api/v1/system/stats/ws?token=${encodeURIComponent(token)}`;

      try {
        ws = new WebSocket(wsUrl);
      } catch (error) {
        console.error("创建 WebSocket 连接失败:", error);
        return;
      }

      ws.onopen = () => {
        dashboardSystemMonitorCache.wsConnected = true;
        setWsConnected(true);
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          const nextStats = normalizeSystemStatsSnapshot(data);
          setSystemStats(nextStats);
          setSystemMetricHistory((previous) => {
            const nextHistory =
              previous.length === 0 || isEmptySystemHistory(previous)
                ? createSystemHistory(nextStats)
                : (() => {
                    const nextPoint: SystemMetricHistoryPoint = {
                      index: previous[previous.length - 1].index + 1,
                      cpu: nextStats.cpu,
                      mem: nextStats.mem,
                      disk: nextStats.disk,
                      network:
                        nextStats.networkRxMbps + nextStats.networkTxMbps,
                    };

                    return [
                      ...previous.slice(-(SYSTEM_HISTORY_LIMIT - 1)),
                      nextPoint,
                    ].map((item, index) => ({
                      ...item,
                      index,
                    }));
                  })();

            dashboardSystemMonitorCache.stats = nextStats;
            dashboardSystemMonitorCache.history = nextHistory;
            return nextHistory;
          });
        } catch (error) {
          console.error("解析系统性能数据失败:", error);
        }
      };

      ws.onerror = () => {
        dashboardSystemMonitorCache.wsConnected = false;
        setWsConnected(false);
      };

      ws.onclose = () => {
        dashboardSystemMonitorCache.wsConnected = false;
        setWsConnected(false);
      };
    };

    const hydrateSystemHistory = async () => {
      try {
        const historyRes = await fetchWithAuth(
          `/api/v1/system/stats/history?limit=${SYSTEM_HISTORY_LIMIT}`,
        ).catch(() => null);
        if (!historyRes?.ok || !isActive) {
          return;
        }

        const payload: SystemStatsHistoryResponse = await historyRes.json();
        if (!isActive || !payload.success) {
          return;
        }

        const latestStats = normalizeSystemStatsSnapshot(payload.data?.latest);
        const historySamples = Array.isArray(payload.data?.samples)
          ? payload
              .data!.samples.slice(-SYSTEM_HISTORY_LIMIT)
              .map((sample, index) =>
                normalizeSystemHistoryPoint(sample, index),
              )
          : [];

        if (historySamples.length > 0) {
          dashboardSystemMonitorCache.history = historySamples;
          setSystemMetricHistory(historySamples);
        } else if (
          (dashboardSystemMonitorCache.history.length === 0 ||
            isEmptySystemHistory(dashboardSystemMonitorCache.history)) &&
          (latestStats.cpu > 0 || latestStats.mem > 0 || latestStats.disk > 0)
        ) {
          const seededHistory = createSystemHistory(latestStats);
          dashboardSystemMonitorCache.history = seededHistory;
          setSystemMetricHistory(seededHistory);
        }

        if (
          latestStats.platform ||
          latestStats.arch ||
          latestStats.os ||
          latestStats.hostname ||
          latestStats.cpu > 0 ||
          latestStats.mem > 0 ||
          latestStats.disk > 0
        ) {
          dashboardSystemMonitorCache.stats = latestStats;
          setSystemStats(latestStats);
        }
      } catch (error) {
        console.error("加载系统性能历史失败:", error);
      }
    };

    void hydrateSystemHistory().finally(() => {
      connectWebSocket();
    });

    return () => {
      isActive = false;
      if (ws) {
        ws.close();
      }
    };
  }, []);

  // 加载所有统计数据
  useEffect(() => {
    if (pathname !== "/dashboard") return;
    const loadDashboardStats = async () => {
      setIsLoading(true);
      try {
        const [
          dashboardTasksRes,
          vulnStatsRes,
          vulnDisposalStatsRes,
          assetStatsRes,
          recentVulnsRes,
        ] = await Promise.all([
          fetchWithAuth(`/api/v1/dashboard/tasks?limit=10`).catch(() => null), // 统一任务接口
          fetchWithAuth(`/api/v1/webscan/vuln-stats`).catch(() => null),
          fetchWithAuth(`/api/v1/dashboard/vuln-disposal-stats`).catch(
            () => null,
          ),
          fetchWithAuth(`/api/v1/dashboard/asset-stats`).catch(() => null), // 资产统计接口
          fetchWithAuth(`/api/v1/webscan/vulns?limit=10`).catch(() => null),
        ]);

        // 处理统一任务数据
        let taskTypeDistribution: Record<string, number> = {
          webscan: 0,
          portscan: 0,
          subdomain: 0,
          dirsearch: 0,
          dirscan: 0,
          jsscan: 0,
          crack: 0,
          combination: 0,
        };
        let taskStatusDistribution: Record<string, number> = {
          completed: 0,
          running: 0,
          failed: 0,
          cancelled: 0,
          pending: 0,
        };
        let dashboardTasks: DashboardTask[] = [];

        if (dashboardTasksRes?.ok) {
          const data = await dashboardTasksRes.json();
          if (data.success && data.data) {
            dashboardTasks = deduplicateDashboardTasks(
              data.data.recentTasks || [],
            );
            taskTypeDistribution = {
              webscan: 0,
              portscan: 0,
              subdomain: 0,
              dirsearch: 0,
              dirscan: 0,
              jsscan: 0,
              crack: 0,
              combination: 0,
              ...data.data.taskTypeDistribution,
            };
            taskStatusDistribution = {
              completed: 0,
              running: 0,
              failed: 0,
              cancelled: 0,
              pending: 0,
              ...data.data.taskStatusDistribution,
            };
          }
        }

        setRecentTasks(dashboardTasks);

        const totalTasks = Object.values(taskTypeDistribution).reduce(
          (sum, count) => sum + count,
          0,
        );
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        const scansToday = dashboardTasks.filter(
          (t) => new Date(t.createdAt) >= today,
        ).length;
        const successRate =
          totalTasks > 0
            ? Math.round(
                ((taskStatusDistribution.completed || 0) / totalTasks) * 100,
              )
            : 0;

        let vulnStats = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
        if (vulnStatsRes?.ok) {
          const data = await vulnStatsRes.json();
          if (data.success && data.data) {
            vulnStats = data.data;
          }
        }

        let vulnDisposalStats = {
          open: 0,
          inProgress: 0,
          fixed: 0,
          falsePositive: 0,
          ignored: 0,
        };
        if (vulnDisposalStatsRes?.ok) {
          const data = await vulnDisposalStatsRes.json();
          if (data.success && data.data) {
            vulnDisposalStats = {
              open: data.data.open || 0,
              inProgress: data.data.inProgress || 0,
              fixed: data.data.fixed || 0,
              falsePositive: data.data.falsePositive || 0,
              ignored: data.data.ignored || 0,
            };
          }
        }

        let portCount = 0;
        let ipCount = 0;
        let websiteCount = 0;
        let subdomainCount = 0;
        if (assetStatsRes?.ok) {
          const data = await assetStatsRes.json();
          if (data.success && data.data) {
            portCount = data.data.portCount || 0;
            ipCount = data.data.ipCount || 0;
            websiteCount = data.data.websiteCount ?? data.data.fingerprintCount ?? 0;
            subdomainCount = data.data.subdomainCount || 0;
          }
        }

        // 处理最近漏洞
        if (recentVulnsRes?.ok) {
          const data = await recentVulnsRes.json();
          if (data.success && data.data?.vulns) {
            setRecentVulns(
              data.data.vulns.map((v: any) => ({
                id: v.id,
                sourceType: v.sourceType,
                vulnName: v.vulnName || v.templateId,
                templateId: v.templateId,
                target: v.target || v.host,
                severity: v.severity || "info",
              })),
            );
          }
        }

        setStats({
          tasks: {
            total: totalTasks,
            byType: taskTypeDistribution as any,
            running: taskStatusDistribution.running || 0,
            completed: taskStatusDistribution.completed || 0,
            failed: taskStatusDistribution.failed || 0,
            canceled: taskStatusDistribution.cancelled || 0,
          },
          vulnerabilities: {
            ...vulnStats,
            total:
              vulnStats.critical +
              vulnStats.high +
              vulnStats.medium +
              vulnStats.low +
              vulnStats.info,
            disposal: vulnDisposalStats,
          },
          assets: {
            ips: ipCount,
            ports: portCount,
            subdomains: subdomainCount,
            websites: websiteCount,
          },
          efficiency: {
            avgDuration: 0,
            successRate,
            scansToday,
          },
        });
      } catch (error) {
        console.error("Failed to load dashboard stats:", error);
      } finally {
        setIsLoading(false);
      }
    };

    loadDashboardStats();
  }, [pathname]);

  // 检查趋势数据是否全部为零
  const hasTrendData = useMemo(() => {
    if (trendData.length === 0) return false;
    return trendData.some(
      (d) => d.critical > 0 || d.high > 0 || d.medium > 0 || d.low > 0,
    );
  }, [trendData]);

  const systemMonitorCards = useMemo(() => {
    const getMetricStatus = (metric: SystemMetricKey, value: number) => {
      const mediumThreshold =
        metric === "cpu" ? 45 : metric === "mem" ? 60 : 68;
      const highThreshold = metric === "cpu" ? 70 : metric === "mem" ? 75 : 75;

      if (value >= highThreshold) {
        return {
          label: t.dashboard.systemMonitor.statusElevated,
          badgeClassName:
            "border-tone-red-border bg-tone-red-soft text-tone-red",
        };
      }

      if (value >= mediumThreshold) {
        return {
          label: t.dashboard.systemMonitor.statusModerate,
          badgeClassName:
            "border-tone-amber-border bg-tone-amber-soft text-tone-amber",
        };
      }

      return {
        label: t.dashboard.systemMonitor.statusNormal,
        badgeClassName:
          "border-tone-green-border bg-tone-green-soft text-tone-green",
      };
    };

    const averageRecentValues = (values: number[], count: number) => {
      const subset = values.slice(-Math.min(count, values.length));
      if (subset.length === 0) {
        return 0;
      }
      return subset.reduce((sum, current) => sum + current, 0) / subset.length;
    };

    const cpuHistory = systemMetricHistory.map((point) => point.cpu);

    return [
      {
        key: "cpu" as const,
        label: t.dashboard.systemMonitor.cpuUsage,
        subtitle: "",
        valueText: `${systemStats.cpu.toFixed(1)}%`,
        icon: Cpu,
        iconClassName: "text-tone-blue",
        chartType: "area" as const,
        chartStroke: "#5ea3ff",
        chartFill: "rgba(94, 163, 255, 0.18)",
        footerItems: [
          t.dashboard.systemMonitor.oneMinuteAverage.replace(
            "{value}",
            `${averageRecentValues(cpuHistory, 20).toFixed(1)}%`,
          ),
          t.dashboard.systemMonitor.fiveMinuteAverage.replace(
            "{value}",
            `${averageRecentValues(cpuHistory, 100).toFixed(1)}%`,
          ),
        ],
        history: systemMetricHistory.map((point) => ({
          index: point.index,
          value: point.cpu,
        })),
      },
      {
        key: "mem" as const,
        label: t.dashboard.systemMonitor.memoryUsage,
        subtitle: "",
        valueText: `${systemStats.mem.toFixed(1)}%`,
        icon: Server,
        iconClassName: "text-tone-purple",
        chartType: "area" as const,
        chartStroke: "#ab6bff",
        chartFill: "rgba(171, 107, 255, 0.20)",
        footerItems: [
          systemStats.memTotalGB > 0
            ? t.dashboard.systemMonitor.memoryUsageSummary
                .replace("{used}", formatStorageGB(systemStats.memUsedGB))
                .replace("{total}", formatStorageGB(systemStats.memTotalGB))
            : t.dashboard.systemMonitor.memoryPressure,
        ],
        history: systemMetricHistory.map((point) => ({
          index: point.index,
          value: point.mem,
        })),
      },
      {
        key: "disk" as const,
        label: t.dashboard.systemMonitor.diskUsage,
        subtitle: "",
        valueText: `${systemStats.disk.toFixed(1)}%`,
        icon: HardDrive,
        iconClassName: "text-tone-amber",
        chartType: "area" as const,
        chartStroke: "#ff9b2f",
        chartFill: "rgba(255, 155, 47, 0.22)",
        footerItems: [
          systemStats.diskTotalGB > 0
            ? t.dashboard.systemMonitor.diskUsageSummary
                .replace("{used}", formatStorageGB(systemStats.diskUsedGB))
                .replace("{total}", formatStorageGB(systemStats.diskTotalGB))
            : t.dashboard.systemMonitor.diskPressure,
        ],
        history: systemMetricHistory.map((point) => ({
          index: point.index,
          value: point.disk,
        })),
      },
      {
        key: "network" as const,
        label: t.dashboard.systemMonitor.networkThroughput,
        subtitle: "",
        valueText: formatThroughput(
          systemStats.networkRxMbps + systemStats.networkTxMbps,
        ),
        icon: ArrowDownUp,
        iconClassName: "text-tone-blue",
        chartType: "area" as const,
        chartStroke: "#7aa8ff",
        chartFill: "rgba(122, 168, 255, 0.18)",
        footerItems: [
          t.dashboard.systemMonitor.networkDownSummary.replace(
            "{value}",
            formatThroughput(systemStats.networkRxMbps),
          ),
          t.dashboard.systemMonitor.networkUpSummary.replace(
            "{value}",
            formatThroughput(systemStats.networkTxMbps),
          ),
        ],
        history: systemMetricHistory.map((point) => ({
          index: point.index,
          value: point.network,
        })),
      },
    ].map((metric) => ({
      ...metric,
      status:
        metric.key === "network"
          ? null
          : getMetricStatus(
              metric.key,
              metric.key === "cpu"
                ? systemStats.cpu
                : metric.key === "mem"
                  ? systemStats.mem
                  : systemStats.disk,
            ),
    }));
  }, [
    systemMetricHistory,
    systemStats.cpu,
    systemStats.disk,
    systemStats.diskTotalGB,
    systemStats.diskUsedGB,
    systemStats.mem,
    systemStats.memTotalGB,
    systemStats.memUsedGB,
    systemStats.networkRxMbps,
    systemStats.networkTxMbps,
    t,
  ]);

  const formatTimeAgo = (dateString: string) => {
    const date = new Date(dateString);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);

    if (diffMins < 1) return t.dashboard.timeAgo.justNow;
    if (diffMins < 60)
      return t.dashboard.timeAgo.minutesAgo.replace("{n}", String(diffMins));
    if (diffHours < 24)
      return t.dashboard.timeAgo.hoursAgo.replace("{n}", String(diffHours));
    if (diffDays < 7)
      return t.dashboard.timeAgo.daysAgo.replace("{n}", String(diffDays));
    return date.toLocaleDateString(undefined, {
      month: "short",
      day: "numeric",
    });
  };

  const getTaskTypeInfo = (task: DashboardTask) => {
    const type = normalizeDashboardTaskType(task.taskType);
    switch (type) {
      case "webscan":
        return {
          label: t.dashboard.taskTypes.vulnScan,
          icon: Globe,
          color: "text-tone-blue",
          route: "/scan/vuln-scan",
        };
      case "portscan":
        return {
          label: t.dashboard.taskTypes.portScan,
          icon: Server,
          color: "text-tone-green",
          route: "/scan/vuln-scan",
        };
      case "subdomain":
        return {
          label: t.dashboard.taskTypes.subdomainCollection,
          icon: Captions,
          color: "text-tone-green",
          route: "/collect/subdomain",
        };
      case "dirscan":
        return {
          label: t.dashboard.taskTypes.dirScan,
          icon: FolderSearch,
          color: "text-tone-purple",
          route: "/scan/vuln-scan",
        };
      case "jsscan":
        return {
          label: t.dashboard.taskTypes.jsScan,
          icon: FileCode,
          color: "text-tone-amber",
          route: "/scan/vuln-scan",
        };
      case "crack":
        return {
          label: t.dashboard.taskTypes.crack,
          icon: Target,
          color: "text-tone-orange",
          route: "/scan/vuln-scan",
        };
      case "combination":
        return {
          label: t.dashboard.taskTypes.combination,
          icon: Layers,
          color: "text-tone-slate",
          route: "/scan/vuln-scan",
        };
      default:
        return {
          label: t.dashboard.taskTypes.scanTask,
          icon: Target,
          color: "text-primary",
          route: "/scan/vuln-scan",
        };
    }
  };

  const getTaskDisplayName = (task: DashboardTask) => {
    if (task.taskName) return task.taskName;
    return `${t.dashboard.recentTasks.task} ${task.id.slice(0, 8)}`;
  };

  // 任务类型分布数据（直接使用后端返回的统计数据）
  const taskTypeDistribution = useMemo(() => {
    const typeConfig: Record<string, { name: string; color: string }> = {
      webscan: { name: t.dashboard.taskTypes.vulnScan, color: "#ef4444" },
      portscan: { name: t.dashboard.taskTypes.portScan, color: "#3b82f6" },
      crack: { name: t.dashboard.taskTypes.crack, color: "#64748b" },
      subdomain: {
        name: t.dashboard.taskTypes.subdomainCollection,
        color: "#10b981",
      },
      dirscan: { name: t.dashboard.taskTypes.dirScan, color: "#f59e0b" },
      jsscan: { name: t.dashboard.taskTypes.jsScan, color: "#8b5cf6" },
      combination: {
        name: t.dashboard.taskTypes.combination,
        color: "#94a3b8",
      },
    };

    const mergedCounts: Record<string, number> = {};
    Object.entries(stats.tasks.byType).forEach(([rawType, count]) => {
      const normalized = normalizeDashboardTaskType(rawType);
      if (normalized === "unknown") return;
      mergedCounts[normalized] = (mergedCounts[normalized] || 0) + count;
    });

    return Object.entries(stats.tasks.byType)
      .filter(([, count]) => count > 0)
      .map(([type]) => normalizeDashboardTaskType(type))
      .filter((type): type is DashboardTaskType => type !== "unknown")
      .filter((type, index, arr) => arr.indexOf(type) === index)
      .map((type) => ({
        name: typeConfig[type]?.name || t.dashboard.taskTypes.scanTask,
        value: mergedCounts[type] || 0,
        color: typeConfig[type]?.color || "#64748b",
      }))
      .filter((item) => item.value > 0)
      .sort((a, b) => b.value - a.value);
  }, [stats.tasks.byType, t]);

  const renderAssetStatValue = (value: number) => (
    <Tooltip>
      <TooltipTrigger asChild>
        <p className="cursor-default text-xl font-bold leading-none text-foreground tabular-nums">
          {formatCompactCount(value)}
        </p>
      </TooltipTrigger>
      <TooltipContent side="top">
        <span>{value.toLocaleString("en-US")}</span>
      </TooltipContent>
    </Tooltip>
  );

  return (
    <div
      className={cn(
        "flex-1 overflow-y-auto bg-background",
        !isLoading && "scan-panel-enter",
        isLoading && "scan-surface-pending",
      )}
      aria-busy={isLoading}
    >
      <div className="space-y-3 p-3 sm:space-y-4 sm:p-4">
        {/* 第一行：资产统计 + 任务统计 */}
        <div className="grid grid-cols-12 gap-3">
          {/* 资产统计卡片 */}
          <Card className="col-span-12 p-3 transition-all group hover:border-tone-blue-border sm:p-4 xl:col-span-6">
            <div className="mb-2 flex min-h-[32px] items-center justify-between">
              <div className="flex items-center gap-2">
                <div className="w-7 h-7 rounded-lg bg-tone-blue-soft border border-tone-blue-border flex items-center justify-center">
                  <Server className="w-3.5 h-3.5 text-tone-blue" />
                </div>
                <span className="text-sm font-medium text-foreground">
                  {t.dashboard.assetStats.title}
                </span>
              </div>
              <span className="invisible text-2xl font-bold leading-none">
                0
              </span>
            </div>
            <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
              <Link
                href="/asset-center?category=ip"
                className="flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border border-tone-blue-border bg-tone-blue-soft p-2 transition-colors hover:bg-tone-blue-soft/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-tone-blue focus-visible:ring-offset-2"
                aria-label={t.dashboard.assetStats.ipAddress}
              >
                <div className="w-8 h-8 rounded-md border border-[var(--tone-blue-icon-border)] bg-[var(--tone-blue-icon-bg)] flex items-center justify-center flex-shrink-0">
                  <Monitor className="w-4 h-4 text-tone-blue" />
                </div>
                <div className="min-w-0 flex-1">
                  {renderAssetStatValue(stats.assets.ips)}
                  <p className="block min-w-0 truncate text-[10px] text-tone-blue">
                    {t.dashboard.assetStats.ipAddress}
                  </p>
                </div>
              </Link>
              <Link
                href="/asset-center?category=port"
                className="flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border border-tone-green-border bg-tone-green-soft p-2 transition-colors hover:bg-tone-green-soft/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-tone-green focus-visible:ring-offset-2"
                aria-label={t.dashboard.assetStats.openPorts}
              >
                <div className="w-8 h-8 rounded-md border border-[var(--tone-green-icon-border)] bg-[var(--tone-green-icon-bg)] flex items-center justify-center flex-shrink-0">
                  <Plug className="w-4 h-4 text-tone-green" />
                </div>
                <div className="min-w-0 flex-1">
                  {renderAssetStatValue(stats.assets.ports)}
                  <p className="block min-w-0 truncate text-[10px] text-tone-green">
                    {t.dashboard.assetStats.openPorts}
                  </p>
                </div>
              </Link>
              <Link
                href="/asset-center?category=subdomain"
                className="flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border border-tone-purple-border bg-tone-purple-soft p-2 transition-colors hover:bg-tone-purple-soft/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-tone-purple focus-visible:ring-offset-2"
                aria-label={t.dashboard.assetStats.subdomains}
              >
                <div className="w-8 h-8 rounded-md border border-[var(--tone-purple-icon-border)] bg-[var(--tone-purple-icon-bg)] flex items-center justify-center flex-shrink-0">
                  <Globe className="w-4 h-4 text-tone-purple" />
                </div>
                <div className="min-w-0 flex-1">
                  {renderAssetStatValue(stats.assets.subdomains)}
                  <p className="block min-w-0 truncate text-[10px] text-tone-purple">
                    {t.dashboard.assetStats.subdomains}
                  </p>
                </div>
              </Link>
              <Link
                href="/asset-center?category=website"
                className="flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border border-tone-amber-border bg-tone-amber-soft p-2 transition-colors hover:bg-tone-amber-soft/80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-tone-amber focus-visible:ring-offset-2"
                aria-label={t.dashboard.assetStats.websites}
              >
                <div className="w-8 h-8 rounded-md border border-[var(--tone-amber-icon-border)] bg-[var(--tone-amber-icon-bg)] flex items-center justify-center flex-shrink-0">
                  <Fingerprint className="w-4 h-4 text-tone-amber" />
                </div>
                <div className="min-w-0 flex-1">
                  {renderAssetStatValue(stats.assets.websites)}
                  <p className="block min-w-0 truncate text-[10px] text-tone-amber">
                    {t.dashboard.assetStats.websites}
                  </p>
                </div>
              </Link>
            </div>
          </Card>

          {/* 任务统计卡片 */}
          <Card className="col-span-12 p-3 transition-all group hover:border-primary/50 sm:p-4 xl:col-span-6">
            <div className="mb-2 flex min-h-[32px] items-center justify-between">
              <div className="flex items-center gap-2">
                <div className="w-7 h-7 rounded-lg bg-primary/20 flex items-center justify-center">
                  <Scan className="w-3.5 h-3.5 text-primary" />
                </div>
                <span className="text-sm font-medium text-foreground">
                  {t.dashboard.taskStats.title}
                </span>
              </div>
              <span className="text-2xl font-bold leading-none text-foreground">
                {stats.tasks.total}
              </span>
            </div>
            <div className="grid grid-cols-2 gap-1.5 lg:grid-cols-4">
              <div className="flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border border-tone-green-border bg-tone-green-soft p-2">
                <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md border border-[var(--tone-green-icon-border)] bg-[var(--tone-green-icon-bg)]">
                  <CheckCircle2 className="h-4 w-4 text-tone-green" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xl font-bold leading-none text-foreground">
                    {stats.tasks.completed}
                  </p>
                  <p className="block min-w-0 truncate text-[10px] text-tone-green">
                    {t.dashboard.taskStats.completed}
                  </p>
                </div>
              </div>
              <div className="flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border border-primary/20 bg-primary/10 p-2">
                <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary/20">
                  <Activity className="h-4 w-4 text-primary" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xl font-bold leading-none text-foreground">
                    {stats.tasks.running}
                  </p>
                  <p className="block min-w-0 truncate text-[10px] text-primary/80">
                    {t.dashboard.taskStats.running}
                  </p>
                </div>
              </div>
              <div className="flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border border-tone-red-border bg-tone-red-soft p-2">
                <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md border border-[var(--tone-red-icon-border)] bg-[var(--tone-red-icon-bg)]">
                  <XCircle className="h-4 w-4 text-tone-red" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xl font-bold leading-none text-foreground">
                    {stats.tasks.failed}
                  </p>
                  <p className="block min-w-0 truncate text-[10px] text-tone-red">
                    {t.dashboard.taskStats.failed}
                  </p>
                </div>
              </div>
              <div className="dashboard-status-muted-surface flex min-w-0 min-h-[58px] items-center gap-2.5 rounded-lg border p-2">
                <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md border border-[var(--tone-slate-icon-border)] bg-[var(--tone-slate-icon-bg)]">
                  <Ban className="dashboard-status-muted-text h-4 w-4" />
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-xl font-bold leading-none text-foreground">
                    {stats.tasks.canceled}
                  </p>
                  <p className="dashboard-status-muted-text block min-w-0 truncate text-[10px]">
                    {t.dashboard.taskStats.cancelled}
                  </p>
                </div>
              </div>
            </div>
          </Card>
        </div>

        {/* 第二行：漏洞统计 */}
        <Card className="p-3 transition-all group hover:border-foreground/20 sm:p-4">
          <Link href="/vulnerabilities" className="mb-3 flex items-center justify-between rounded-lg transition-[filter,box-shadow] hover:brightness-110 [.shadcn-light_&]:hover:filter-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
            <div className="flex items-center gap-2">
              <div className="w-7 h-7 rounded-lg bg-tone-red-soft border border-tone-red-border flex items-center justify-center">
                <Bug className="w-3.5 h-3.5 text-tone-red" />
              </div>
              <span className="text-sm font-medium text-foreground">
                {t.dashboard.vulnStats.title}
              </span>
            </div>
            <span className="text-2xl font-bold text-foreground">
              {stats.vulnerabilities.total}
            </span>
          </Link>
          <div className="grid gap-4 lg:grid-cols-2">
            <div>
              <div className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
                <ShieldAlert className="h-3.5 w-3.5" />
                <span>{t.dashboard.vulnStats.severityTitle}</span>
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-5 sm:gap-1.5">
                <Link href="/vulnerabilities?severity=critical" className="flex min-h-[58px] items-center rounded-lg border border-tone-red-border bg-tone-red-soft p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.critical}
                    </p>
                    <p className="truncate text-[10px] text-tone-red">
                      {t.dashboard.vulnStats.critical}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?severity=high" className="flex min-h-[58px] items-center rounded-lg border border-tone-orange-border bg-tone-orange-soft p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.high}
                    </p>
                    <p className="truncate text-[10px] text-tone-orange">
                      {t.dashboard.vulnStats.high}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?severity=medium" className="flex min-h-[58px] items-center rounded-lg border border-tone-amber-border bg-tone-amber-soft p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.medium}
                    </p>
                    <p className="truncate text-[10px] text-tone-amber">
                      {t.dashboard.vulnStats.medium}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?severity=low" className="flex min-h-[58px] items-center rounded-lg border border-tone-blue-border bg-tone-blue-soft p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.low}
                    </p>
                    <p className="truncate text-[10px] text-tone-blue">
                      {t.dashboard.vulnStats.low}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?severity=info" className="dashboard-severity-info-surface flex min-h-[58px] items-center rounded-lg border p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.info}
                    </p>
                    <p className="dashboard-severity-info-text truncate text-[10px]">
                      {t.dashboard.vulnStats.info}
                    </p>
                  </div>
                </Link>
              </div>
            </div>
            <div>
              <div className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
                <ListChecks className="h-3.5 w-3.5" />
                <span>{t.dashboard.vulnStats.disposalTitle}</span>
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-5 sm:gap-1.5">
                <Link href="/vulnerabilities?status=open" className="dashboard-status-open-surface flex min-h-[58px] items-center rounded-lg border p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.disposal.open}
                    </p>
                    <p className="dashboard-status-open-text truncate text-[10px]">
                      {t.dashboard.vulnStats.open}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?status=in_progress" className="flex min-h-[58px] items-center rounded-lg border border-tone-blue-border bg-tone-blue-soft p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.disposal.inProgress}
                    </p>
                    <p className="truncate text-[10px] text-tone-blue">
                      {t.dashboard.vulnStats.inProgress}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?status=fixed" className="flex min-h-[58px] items-center rounded-lg border border-tone-green-border bg-tone-green-soft p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.disposal.fixed}
                    </p>
                    <p className="truncate text-[10px] text-tone-green">
                      {t.dashboard.vulnStats.fixed}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?status=false_positive" className="flex min-h-[58px] items-center rounded-lg border border-tone-amber-border bg-tone-amber-soft p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.disposal.falsePositive}
                    </p>
                    <p className="truncate text-[10px] text-tone-amber">
                      {t.dashboard.vulnStats.falsePositive}
                    </p>
                  </div>
                </Link>
                <Link href="/vulnerabilities?status=ignored" className="dashboard-status-muted-surface flex min-h-[58px] items-center rounded-lg border p-2 text-center transition-[filter,box-shadow] hover:brightness-110 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-foreground/30 focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                  <div className="min-w-0 flex-1">
                    <p className="text-xl font-bold leading-none text-foreground">
                      {stats.vulnerabilities.disposal.ignored}
                    </p>
                    <p className="dashboard-status-muted-text truncate text-[10px]">
                      {t.dashboard.vulnStats.ignored}
                    </p>
                  </div>
                </Link>
              </div>
            </div>
          </div>
        </Card>

        {/* 第三行：最近漏洞 + 最近扫描活动 */}
        <div className="grid grid-cols-12 gap-3">
          {/* 最近发现的漏洞 */}
          <Card className="col-span-12 lg:col-span-7 p-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-foreground flex items-center gap-2">
                <Bug className="w-4 h-4 text-tone-red" />
                {t.dashboard.recentVulns.title}
              </h3>
            </div>
            <div className="space-y-2 max-h-[200px] overflow-y-auto pr-1">
              {recentVulns.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                  <Shield className="w-10 h-10 mb-2 opacity-30" />
                  <p className="text-sm">{t.dashboard.recentVulns.noVulns}</p>
                  <p className="text-xs opacity-60 mt-1">
                    {t.dashboard.recentVulns.systemSafe}
                  </p>
                </div>
              ) : (
                recentVulns.slice(0, 10).map((vuln, index) => {
                  const severity = vuln.severity?.toLowerCase() || "info";
                  const isAISession = vuln.sourceType === "pentest_blackboard";
                  const severityConfig: Record<
                    string,
                    { dot: string; badge: string; label: string }
                  > = {
                    critical: {
                      dot: "bg-tone-red",
                      badge:
                        "text-tone-red border-tone-red-border bg-tone-red-soft",
                      label: t.dashboard.vulnStats.critical,
                    },
                    high: {
                      dot: "bg-tone-orange",
                      badge:
                        "text-tone-orange border-tone-orange-border bg-tone-orange-soft",
                      label: t.dashboard.vulnStats.high,
                    },
                    medium: {
                      dot: "bg-tone-amber",
                      badge:
                        "text-tone-amber border-tone-amber-border bg-tone-amber-soft",
                      label: t.dashboard.vulnStats.medium,
                    },
                    low: {
                      dot: "bg-tone-blue",
                      badge:
                        "text-tone-blue border-tone-blue-border bg-tone-blue-soft",
                      label: t.dashboard.vulnStats.low,
                    },
                    info: {
                      dot: "bg-tone-slate",
                      badge:
                        "text-tone-slate border-tone-slate-border bg-tone-slate-soft",
                      label: t.dashboard.vulnStats.info,
                    },
                  };
                  const config =
                    severityConfig[severity] || severityConfig.info;

                  return (
                    <div
                      key={`${vuln.sourceType}-${vuln.id}-${index}`}
                      className="flex items-center gap-3 p-2.5 rounded-lg"
                    >
                      <div
                        className={cn(
                          "w-2 h-2 rounded-full flex-shrink-0",
                          config.dot,
                        )}
                      />
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-1.5 min-w-0">
                          {isAISession && vuln.id ? (
                            <Link
                              href={findingDetailHref(vuln.id, "/dashboard")}
                              className="text-sm font-medium text-foreground truncate rounded-sm hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-muted-foreground"
                            >
                              {vuln.vulnName || vuln.templateId}
                            </Link>
                          ) : (
                            <p className="text-sm font-medium text-foreground truncate">
                              {vuln.vulnName || vuln.templateId}
                            </p>
                          )}
                          {isAISession && (
                            <span className="inline-flex h-5 flex-shrink-0 items-center gap-1 rounded-md border border-border bg-muted/40 px-1.5 text-[11px] font-medium leading-none text-muted-foreground">
                              <Bot className="h-3 w-3" />
                              {t.dashboard.recentVulns.aiSession}
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-muted-foreground truncate">
                          {vuln.target}
                        </p>
                      </div>
                      <Badge
                        variant="outline"
                        className={cn("text-xs flex-shrink-0", config.badge)}
                      >
                        {config.label}
                      </Badge>
                    </div>
                  );
                })
              )}
            </div>
          </Card>

          {/* 最近扫描活动 */}
          <Card className="col-span-12 lg:col-span-5 p-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-foreground flex items-center gap-2">
                <Activity className="w-4 h-4 text-primary" />
                {t.dashboard.recentTasks.title}
              </h3>
            </div>
            <div className="space-y-1.5 max-h-[200px] overflow-y-auto pr-1">
              {recentTasks.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                  <Scan className="w-10 h-10 mb-2 opacity-30" />
                  <p className="text-sm">{t.dashboard.recentTasks.noTasks}</p>
                </div>
              ) : (
                recentTasks.map((task) => {
                  const typeInfo = getTaskTypeInfo(task);
                  const StatusIcon =
                    STATUS_COLORS[task.status as keyof typeof STATUS_COLORS]
                      ?.icon || Clock;
                  const statusColor =
                    STATUS_COLORS[task.status as keyof typeof STATUS_COLORS] ||
                    STATUS_COLORS.pending;

                  return (
                    <div
                      key={`${task.taskType}-${task.id}`}
                      className="flex items-center gap-3 p-2.5 rounded-lg hover:bg-muted/50 transition-colors group cursor-pointer"
                      onClick={() => router.push(typeInfo.route)}
                    >
                      <div
                        className={cn(
                          "w-8 h-8 rounded-lg flex items-center justify-center",
                          statusColor.bg,
                        )}
                      >
                        <StatusIcon
                          className={cn(
                            "w-4 h-4",
                            statusColor.text,
                            task.status === "running" && "animate-pulse",
                          )}
                        />
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium text-foreground truncate">
                          {getTaskDisplayName(task)}
                        </p>
                        <div className="flex items-center gap-2 text-xs text-muted-foreground mt-0.5">
                          <typeInfo.icon
                            className={cn("w-3 h-3", typeInfo.color)}
                          />
                          <span>{typeInfo.label}</span>
                          <span className="opacity-50">·</span>
                          <span>{formatTimeAgo(task.updatedAt)}</span>
                        </div>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </Card>
        </div>

        {/* 第四行：漏洞趋势 + 任务类型分布 */}
        <div className="grid grid-cols-12 gap-3 items-stretch">
          {/* 漏洞趋势图 */}
          <Card className="col-span-12 lg:col-span-8 p-4">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-semibold text-foreground flex items-center gap-2">
                <TrendingUp className="w-4 h-4 text-primary" />
                {t.dashboard.vulnTrend.title}
              </h3>
              <DashboardDateRange
                value={vulnTrendDays}
                onChange={setVulnTrendDays}
                dayLabel={t.dashboard.vulnTrend.days}
              />
            </div>
            <div className="h-[220px]">
              {trendLoading ? (
                <div className="h-full flex items-center justify-center">
                  <div className="flex flex-col items-center gap-3">
                    <div className="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin" />
                    <span className="text-xs text-muted-foreground">
                      {t.dashboard.vulnTrend.loading}
                    </span>
                  </div>
                </div>
              ) : !hasTrendData ? (
                <div className="h-full flex items-center justify-center border border-dashed border-border rounded-lg">
                  <div className="flex flex-col items-center gap-2 text-muted-foreground">
                    <TrendingUp className="w-10 h-10 opacity-20" />
                    <p className="text-sm">{t.dashboard.vulnTrend.noData}</p>
                    <p className="text-xs opacity-60">
                      {t.dashboard.vulnTrend.noVulnsInDays.replace(
                        "{days}",
                        String(vulnTrendDays),
                      )}
                    </p>
                  </div>
                </div>
              ) : (
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart
                    data={trendData}
                    margin={{ top: 10, right: 10, left: -20, bottom: 0 }}
                  >
                    <defs>
                      <linearGradient
                        id="gradCritical"
                        x1="0"
                        y1="0"
                        x2="0"
                        y2="1"
                      >
                        <stop
                          offset="0%"
                          stopColor="#ef4444"
                          stopOpacity={0.5}
                        />
                        <stop
                          offset="100%"
                          stopColor="#ef4444"
                          stopOpacity={0}
                        />
                      </linearGradient>
                      <linearGradient id="gradHigh" x1="0" y1="0" x2="0" y2="1">
                        <stop
                          offset="0%"
                          stopColor="#f97316"
                          stopOpacity={0.5}
                        />
                        <stop
                          offset="100%"
                          stopColor="#f97316"
                          stopOpacity={0}
                        />
                      </linearGradient>
                      <linearGradient
                        id="gradMedium"
                        x1="0"
                        y1="0"
                        x2="0"
                        y2="1"
                      >
                        <stop
                          offset="0%"
                          stopColor="#f59e0b"
                          stopOpacity={0.5}
                        />
                        <stop
                          offset="100%"
                          stopColor="#f59e0b"
                          stopOpacity={0}
                        />
                      </linearGradient>
                    </defs>
                    <CartesianGrid
                      strokeDasharray="3 3"
                      opacity={0.2}
                      vertical={false}
                    />
                    <XAxis
                      dataKey="name"
                      tick={{ fontSize: 11 }}
                      axisLine={false}
                      tickLine={false}
                    />
                    <YAxis
                      tick={{ fontSize: 11 }}
                      axisLine={false}
                      tickLine={false}
                      allowDecimals={false}
                    />
                    <RechartsTooltip
                      contentStyle={{
                        backgroundColor: "hsl(var(--popover))",
                        border: "1px solid hsl(var(--border))",
                        borderRadius: "8px",
                        boxShadow: "0 4px 12px rgba(0,0,0,0.3)",
                      }}
                      itemStyle={{
                        color: "hsl(var(--foreground))",
                        fontSize: 12,
                      }}
                      labelStyle={{
                        color: "hsl(var(--foreground))",
                        fontWeight: 600,
                        marginBottom: 4,
                      }}
                    />
                    <Area
                      type="monotone"
                      dataKey="critical"
                      name={t.dashboard.vulnStats.critical}
                      stroke="#ef4444"
                      strokeWidth={2}
                      fill="url(#gradCritical)"
                    />
                    <Area
                      type="monotone"
                      dataKey="high"
                      name={t.dashboard.vulnStats.high}
                      stroke="#f97316"
                      strokeWidth={2}
                      fill="url(#gradHigh)"
                    />
                    <Area
                      type="monotone"
                      dataKey="medium"
                      name={t.dashboard.vulnStats.medium}
                      stroke="#f59e0b"
                      strokeWidth={2}
                      fill="url(#gradMedium)"
                    />
                  </AreaChart>
                </ResponsiveContainer>
              )}
            </div>
          </Card>

          {/* 任务类型分布 */}
          <Card className="col-span-12 md:col-span-6 lg:col-span-4 p-4">
            <h3 className="text-sm font-semibold text-foreground flex items-center gap-2 mb-3">
              <Layers className="w-4 h-4 text-primary" />
              {t.dashboard.taskDistribution.title}
            </h3>
            <div className="flex h-[220px] flex-col justify-start pt-2">
              {taskTypeDistribution.length === 0 ? (
                <div className="flex h-full items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground">
                  {t.dashboard.noData}
                </div>
              ) : (
                <div className="space-y-2.5">
                  {taskTypeDistribution.map((item) => {
                    const maxValue = taskTypeDistribution[0]?.value || 1;
                    const barWidth = Math.max(
                      12,
                      (item.value / maxValue) * 100,
                    );

                    return (
                      <div
                        key={item.name}
                        className="grid grid-cols-[minmax(0,92px)_minmax(0,1fr)_30px] items-center gap-3"
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          <div
                            className="h-2.5 w-2.5 shrink-0 rounded-full"
                            style={{ backgroundColor: item.color }}
                          />
                          <span
                            className="truncate text-xs text-muted-foreground"
                            title={item.name}
                          >
                            {item.name}
                          </span>
                        </div>
                        <div className="h-2.5 overflow-hidden rounded-full bg-muted/70 dark:bg-muted/45">
                          <div
                            className="h-full rounded-full transition-all"
                            style={{
                              width: `${barWidth}%`,
                              background: `linear-gradient(90deg, ${item.color}, ${item.color}CC)`,
                            }}
                          />
                        </div>
                        <span className="text-right text-xs font-medium tabular-nums text-foreground">
                          {item.value}
                        </span>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </Card>
        </div>

        {/* Token 使用汇总 */}
        <Card className="p-4 transition-all hover:border-tone-blue-border">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
              <Gauge className="h-4 w-4 text-tone-blue" />
              {t.dashboard.tokenUsage.title}
            </h3>
            <DashboardDateRange
              value={tokenUsageDays}
              onChange={setTokenUsageDays}
              dayLabel={t.dashboard.vulnTrend.days}
            />
          </div>
          <div className="grid gap-5 lg:grid-cols-[318px_minmax(0,1fr)] lg:items-stretch">
            <div className="min-w-0">
              <p className="text-[11px] font-medium text-muted-foreground">
                {t.dashboard.tokenUsage.rangeTotal.replace(
                  "{days}",
                  String(tokenUsageDays),
                )}
              </p>
              <p className="mt-1.5 text-3xl font-bold leading-none tracking-tight text-foreground tabular-nums">
                {formatCompactCount(tokenUsageSummary.total)}
              </p>
              <p className="mt-1.5 text-[11px] text-muted-foreground">
                {t.dashboard.tokenUsage.cumulative}{" "}
                {tokenUsage.totalTokens.toLocaleString("en-US")} ·{" "}
                {t.tokenUsage.recordedOnly}
              </p>
              <div
                className="mt-4 flex h-2 overflow-hidden rounded-full bg-muted"
                role="img"
                aria-label={`${t.dashboard.tokenUsage.inputShort} ${(tokenUsageSummary.inputShare * 100).toFixed(1)}%, ${t.dashboard.tokenUsage.outputShort} ${(tokenUsageSummary.outputShare * 100).toFixed(1)}%`}
              >
                <span
                  className="h-full bg-tone-blue"
                  style={{ width: `${tokenUsageSummary.inputShare * 100}%` }}
                />
                <span
                  className="h-full bg-tone-amber"
                  style={{ width: `${tokenUsageSummary.outputShare * 100}%` }}
                />
                {tokenUsageSummary.history > 0 ? (
                  <span
                    className="h-full bg-muted-foreground/30"
                    style={{ width: `${tokenUsageSummary.historyShare * 100}%` }}
                  />
                ) : null}
              </div>
              <div className="mt-3 space-y-2 text-[11.5px]">
                <div className="flex items-center gap-2">
                  <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-tone-blue" />
                  <span className="text-muted-foreground">
                    {t.dashboard.tokenUsage.inputShort}
                  </span>
                  <span className="font-semibold text-foreground tabular-nums">
                    {formatCompactCount(tokenUsageSummary.input)}
                  </span>
                  <span className="ml-auto text-muted-foreground tabular-nums">
                    {(tokenUsageSummary.inputShare * 100).toFixed(1)}%
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-tone-amber" />
                  <span className="text-muted-foreground">
                    {t.dashboard.tokenUsage.outputShort}
                  </span>
                  <span className="font-semibold text-foreground tabular-nums">
                    {formatCompactCount(tokenUsageSummary.output)}
                  </span>
                  <span className="ml-auto text-muted-foreground tabular-nums">
                    {(tokenUsageSummary.outputShare * 100).toFixed(1)}%
                  </span>
                </div>
                {tokenUsageSummary.history > 0 ? (
                  <div className="flex items-center gap-2">
                    <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-muted-foreground/40" />
                    <span className="text-muted-foreground">
                      {t.dashboard.tokenUsage.historyOnly}
                    </span>
                    <span className="font-semibold text-foreground tabular-nums">
                      {formatCompactCount(tokenUsageSummary.history)}
                    </span>
                    <span className="ml-auto text-muted-foreground tabular-nums">
                      {(tokenUsageSummary.historyShare * 100).toFixed(1)}%
                    </span>
                  </div>
                ) : null}
              </div>
              {tokenUsageSummary.cache > 0 ? (
                <div className="mt-3 flex items-center gap-2 rounded-lg border border-tone-green-border bg-tone-green-soft px-2.5 py-1.5 text-[11px]">
                  <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-tone-green" />
                  <span className="text-muted-foreground">
                    {t.tokenUsage.cacheRead}
                  </span>
                  <span className="font-semibold text-foreground tabular-nums">
                    {formatCompactCount(tokenUsageSummary.cache)}
                  </span>
                  {tokenUsageSummary.input > 0 ? (
                    <span className="ml-auto text-muted-foreground tabular-nums">
                      {t.dashboard.tokenUsage.cacheShare.replace(
                        "{percent}",
                        (tokenUsageSummary.cacheInputShare * 100).toFixed(1),
                      )}
                    </span>
                  ) : null}
                </div>
              ) : null}
            </div>
            <div className="flex min-w-0 flex-col">
              <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
                <span className="text-[11px] text-muted-foreground">
                  {t.dashboard.tokenUsage.dailyUsage}
                </span>
                <div className="flex items-center gap-3 text-[11px] text-muted-foreground">
                  <span className="flex items-center gap-1.5">
                    <span className="h-1.5 w-1.5 rounded-full bg-tone-blue" />
                    {t.dashboard.tokenUsage.inputShort}
                  </span>
                  <span className="flex items-center gap-1.5">
                    <span className="h-1.5 w-1.5 rounded-full bg-tone-amber" />
                    {t.dashboard.tokenUsage.outputShort}
                  </span>
                  {tokenUsageSummary.history > 0 ? (
                    <span className="flex items-center gap-1.5">
                      <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/40" />
                      {t.dashboard.tokenUsage.historyOnly}
                    </span>
                  ) : null}
                </div>
              </div>
              <div className="h-[200px] min-h-0 w-full text-muted-foreground">
                {tokenUsageLoading ? (
                  <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
                    {t.dashboard.tokenUsage.loading}
                  </div>
                ) : !tokenUsageSummary.hasUsage ? (
                  <div className="flex h-full items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground">
                    {t.dashboard.tokenUsage.noData}
                  </div>
                ) : (
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={tokenUsageSummary.chartData}
                      margin={{ top: 8, right: 4, left: 0, bottom: 0 }}
                      barCategoryGap="28%"
                    >
                      <CartesianGrid
                        vertical={false}
                        strokeDasharray="3 4"
                        stroke="var(--border)"
                        opacity={0.7}
                      />
                      <XAxis
                        dataKey="date"
                        tick={{ fontSize: 10, fill: "currentColor" }}
                        tickFormatter={(value) => String(value).slice(5)}
                        tickLine={false}
                        axisLine={false}
                        tickMargin={6}
                        interval={
                          tokenUsageDays > 14
                            ? Math.ceil(tokenUsageDays / 10)
                            : 0
                        }
                      />
                      <YAxis
                        tick={{ fontSize: 10, fill: "currentColor" }}
                        tickFormatter={(value) =>
                          formatCompactCount(Number(value))
                        }
                        tickLine={false}
                        axisLine={false}
                        width={42}
                      />
                      <RechartsTooltip
                        cursor={{ fill: "var(--muted)", opacity: 0.6 }}
                        content={
                          <TokenUsageTooltip
                            messages={{
                              total: t.tokenUsage.total,
                              input: t.dashboard.tokenUsage.inputShort,
                              output: t.dashboard.tokenUsage.outputShort,
                              history: t.dashboard.tokenUsage.historyOnly,
                              cache: t.tokenUsage.cacheRead,
                            }}
                          />
                        }
                      />
                      <Bar
                        dataKey="inputTokens"
                        stackId="token"
                        style={{ fill: "var(--tone-blue)" }}
                        radius={[2, 2, 0, 0]}
                        maxBarSize={26}
                        isAnimationActive={false}
                      />
                      <Bar
                        dataKey="outputTokens"
                        stackId="token"
                        style={{ fill: "var(--tone-amber)" }}
                        radius={[2, 2, 0, 0]}
                        maxBarSize={26}
                        isAnimationActive={false}
                      />
                      {tokenUsageSummary.history > 0 ? (
                        <Bar
                          dataKey="historyTokens"
                          stackId="token"
                          style={{
                            fill: "var(--muted-foreground)",
                            fillOpacity: 0.32,
                          }}
                          radius={[2, 2, 0, 0]}
                          maxBarSize={26}
                          isAnimationActive={false}
                        />
                      ) : null}
                    </BarChart>
                  </ResponsiveContainer>
                )}
              </div>
            </div>
          </div>
        </Card>

        {/* 第四行：服务器性能监控 */}
        <div className="grid grid-cols-12 gap-3">
          <Card className="col-span-12 overflow-hidden border-border/60 bg-card p-4 shadow-[0_18px_42px_-28px_rgba(15,23,42,0.18)] dark:shadow-[0_18px_42px_-28px_rgba(0,0,0,0.35)]">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
              <h3 className="flex shrink-0 items-center gap-2 text-sm font-semibold text-foreground">
                <Monitor className="w-4 h-4 text-primary" />
                {t.dashboard.systemMonitor.title}
              </h3>
              <div className="ml-auto flex max-w-full flex-wrap items-center justify-end gap-2">
                <span
                  className="max-w-full truncate rounded-full border border-border/60 bg-muted/40 px-3 py-1.5 text-xs font-medium text-muted-foreground"
                  title={`${systemStats.os || t.dashboard.systemMonitor.loading} / ${systemStats.arch || t.dashboard.systemMonitor.loading}`}
                >
                  {systemStats.os || t.dashboard.systemMonitor.loading} /{" "}
                  {systemStats.arch || t.dashboard.systemMonitor.loading}
                </span>
                <div className="flex shrink-0 items-center gap-2 rounded-full border border-tone-green-border bg-tone-green-soft px-3 py-1.5">
                  <div
                    className={cn(
                      "h-2 w-2 rounded-full",
                      wsConnected
                        ? "bg-tone-green animate-pulse"
                        : "bg-tone-slate",
                    )}
                  />
                  <span className="text-xs font-medium text-tone-green">
                    {wsConnected
                      ? t.dashboard.systemMonitor.connected
                      : t.dashboard.systemMonitor.disconnected}
                  </span>
                </div>
              </div>
            </div>
            <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
              {systemMonitorCards.map((metric) => {
                const Icon = metric.icon;
                const historyValues = metric.history.map(
                  (point) => point.value,
                );
                const [domainMin, domainMax] =
                  getSparklineDomain(historyValues);

                return (
                  <div
                    key={metric.key}
                    className="rounded-[18px] border border-border/60 bg-background px-4 py-3.5 shadow-[0_12px_28px_-26px_rgba(15,23,42,0.18)] dark:bg-muted/20 dark:shadow-[0_12px_28px_-26px_rgba(0,0,0,0.28)]"
                  >
                    <div className="mb-3 flex items-start justify-between gap-3">
                      <div className="flex items-center gap-3">
                        <Icon
                          className={cn(
                            "h-[18px] w-[18px] shrink-0",
                            metric.iconClassName,
                          )}
                        />
                        <div className="space-y-0.5">
                          <div className="text-sm font-semibold text-foreground">
                            {metric.label}
                          </div>
                          {metric.subtitle ? (
                            <div className="text-xs text-muted-foreground">
                              {metric.subtitle}
                            </div>
                          ) : null}
                        </div>
                      </div>
                      {metric.status ? (
                        <Badge
                          variant="outline"
                          className={cn(
                            "rounded-full border px-2.5 py-0.5 text-xs font-medium",
                            metric.status.badgeClassName,
                          )}
                        >
                          {metric.status.label}
                        </Badge>
                      ) : null}
                    </div>

                    <div className="mb-3">
                      <div className="text-[32px] font-semibold leading-none tracking-[-0.04em] text-foreground">
                        {metric.valueText}
                      </div>
                    </div>

                    <div className="h-[70px] overflow-hidden rounded-[18px] bg-muted/40 dark:bg-muted/30">
                      <ResponsiveContainer width="100%" height="100%">
                        <AreaChart
                          data={metric.history}
                          margin={{ top: 10, right: 0, left: 0, bottom: 2 }}
                        >
                          <defs>
                            <linearGradient
                              id={`system-metric-${metric.key}`}
                              x1="0"
                              y1="0"
                              x2="0"
                              y2="1"
                            >
                              <stop
                                offset="0%"
                                stopColor={metric.chartFill}
                                stopOpacity={1}
                              />
                              <stop
                                offset="100%"
                                stopColor={metric.chartFill}
                                stopOpacity={0.15}
                              />
                            </linearGradient>
                          </defs>
                          <Area
                            type="monotone"
                            dataKey="value"
                            stroke={metric.chartStroke}
                            strokeWidth={2.5}
                            fill={`url(#system-metric-${metric.key})`}
                            fillOpacity={1}
                            dot={false}
                            isAnimationActive={false}
                          />
                          <YAxis domain={[domainMin, domainMax]} hide />
                        </AreaChart>
                      </ResponsiveContainer>
                    </div>

                    <div
                      className={cn(
                        "mt-3 flex items-center text-[13px] text-muted-foreground",
                        metric.footerItems.length > 1
                          ? "justify-between gap-4"
                          : "justify-start",
                      )}
                    >
                      {metric.footerItems.map((item) => (
                        <span
                          key={`${metric.key}-${item}`}
                          className="truncate"
                        >
                          {item}
                        </span>
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
