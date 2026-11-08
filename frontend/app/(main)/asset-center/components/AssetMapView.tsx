'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Link from 'next/link'
import {
  AlertCircle,
  ArrowUpRight,
  ChevronRight,
  Crosshair,
  Globe2,
  HelpCircle,
  Layers3,
  MapPinned,
  RefreshCw,
  Search,
  Server,
  ShieldAlert,
  Wifi,
  X,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { fetchWithAuth } from '@/lib/api'
import { CITY_COORDINATES } from '@/lib/asset-map/city-coordinates'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import toast from '@/lib/toast'

import { AssetDetailSheet } from './AssetDetailSheet'

/* -------------------------------------------------------------------------- */
/* MapLibre (loaded from the locally bundled CSP build)                        */
/* -------------------------------------------------------------------------- */

type MapLibreMap = {
  addControl: (control: unknown, position?: string) => void
  addLayer: (layer: Record<string, unknown>, beforeId?: string) => void
  addSource: (id: string, source: Record<string, unknown>) => void
  flyTo: (options: { center: [number, number]; zoom: number; duration: number }) => void
  getCanvas: () => HTMLCanvasElement
  getLayer: (id: string) => unknown
  getSource: (id: string) => { setData?: (data: unknown) => void } | undefined
  getZoom: () => number
  on: (
    event: string,
    layerOrCallback: string | ((event: Record<string, unknown>) => void),
    callback?: (event: Record<string, unknown>) => void
  ) => void
  setFilter: (layer: string, filter: unknown) => void
  setLayoutProperty: (layer: string, property: string, value: unknown) => void
  setPaintProperty: (layer: string, property: string, value: unknown) => void
  resize?: () => void
  remove: () => void
}

type MapLibrePopup = {
  setLngLat: (lngLat: [number, number]) => MapLibrePopup
  setHTML: (html: string) => MapLibrePopup
  addTo: (map: MapLibreMap) => MapLibrePopup
  remove: () => void
}

type MapLibreMarker = {
  setLngLat: (lngLat: [number, number]) => MapLibreMarker
  addTo: (map: MapLibreMap) => MapLibreMarker
  remove: () => void
}

type MapLibreGlobal = {
  Map: new (options: Record<string, unknown>) => MapLibreMap
  NavigationControl: new (options?: Record<string, unknown>) => unknown
  Popup: new (options?: Record<string, unknown>) => MapLibrePopup
  Marker: new (options?: Record<string, unknown>) => MapLibreMarker
  setWorkerUrl?: (url: string) => void
}

declare global {
  interface Window {
    maplibregl?: MapLibreGlobal
  }
}

/* -------------------------------------------------------------------------- */
/* API types                                                                  */
/* -------------------------------------------------------------------------- */

type DimensionStat = { name: string; count: number }

type AssetMapRegion = {
  province: string
  provinceLabel: string
  assetCount: number
  taskCount: number
  criticalCount: number
  highCount: number
  mediumCount: number
  directCritical?: number
  directHigh?: number
  inheritedCritical?: number
  inheritedHigh?: number
  inheritedMedium?: number
  directMedium?: number
  cities: DimensionStat[]
  operators: DimensionStat[]
  asns: DimensionStat[]
}

type AssetMapCountry = {
  name: string
  country: string
  countryLabel: string
  count: number
  assetCount: number
  taskCount: number
  criticalCount: number
  highCount: number
  mediumCount: number
  directCritical?: number
  directHigh?: number
  inheritedCritical?: number
  inheritedHigh?: number
  inheritedMedium?: number
  directMedium?: number
  cities: DimensionStat[]
  operators: DimensionStat[]
  asns: DimensionStat[]
}

type AssetMapOverview = {
  totalIpAssets: number
  locatedIpAssets: number
  unknownIpAssets: number
  overseasIpAssets: number
  nonPublicIpAssets: number
  unresolvedIpAssets: number
  totalCritical: number
  totalHigh: number
  totalMedium: number
  riskAssets?: {
    total: number
    ip: number
    domain: number
    url: number
    service: number
    certificate: number
    critical: number
    high: number
    medium: number
  }
  updatedAt?: string
  regions: AssetMapRegion[]
  operators: DimensionStat[]
  asns: DimensionStat[]
  countries: AssetMapCountry[]
}

type AssetMapAsset = {
  assetId?: string
  value: string
  location: string
  province?: string
  provinceLabel?: string
  city?: string
  country?: string
  operator: string
  asn: string
  taskCount: number
  criticalCount: number
  highCount: number
  mediumCount: number
  directCritical?: number
  directHigh?: number
  directMedium?: number
  inheritedCritical?: number
  inheritedHigh?: number
  inheritedMedium?: number
  ports?: string[]
  services?: string[]
  lastSeenAt?: string
  updatedAt: string
}

type AssetMapAssetList = {
  items: AssetMapAsset[]
  total: number
  page: number
  pageSize: number
}

type MapScope = 'located' | 'internal' | 'unresolved' | 'overseas'
type MapMode = 'china' | 'world'
type MapColorMode = 'assets' | 'risk'
type RiskFilter = 'all' | 'critical' | 'high' | 'medium' | 'any'
type DetailTab = 'assets' | 'cities' | 'operators'

type GeoFeature = {
  type: 'Feature'
  properties: Record<string, unknown>
  geometry: unknown
}

type GeoCollection = {
  type: 'FeatureCollection'
  features: GeoFeature[]
}

type Palette = {
  scale: string[]
  landLine: string
  worldFill: string
  worldLine: string
  city: string
  citySelected: string
  selectedFill: string
  selectedLine: string
  labelStrong: string
  dotStroke: string
}

/* -------------------------------------------------------------------------- */
/* Constants and helpers                                                      */
/* -------------------------------------------------------------------------- */

const municipalityProvinces = new Set(['北京', '天津', '上海', '重庆', '香港', '澳门'])

const chinaCenter: [number, number] = [104.1954, 35.8617]
const worldCenter: [number, number] = [15, 22]
const chinaGeoURL = '/geo/china-provinces.json'
const worldGeoURL = '/geo/world-countries.json'
const mapLibreAssetBase = '/maplibre-gl'
const mapLibreCSSURL = `${mapLibreAssetBase}/maplibre-gl.css`
const mapLibreScriptURL = `${mapLibreAssetBase}/maplibre-gl-csp.js`
const mapLibreWorkerURL = `${mapLibreAssetBase}/maplibre-gl-csp-worker.js`
const mapLibreLoadTimeoutMS = 15000

const blankMapStyle = {
  version: 8,
  sources: {},
  layers: [
    {
      id: 'background',
      type: 'background',
      paint: { 'background-color': 'rgba(0, 0, 0, 0)' },
    },
  ],
}

const countryCoordinates: Record<string, [number, number]> = {
  美国: [-98.5795, 39.8283],
  日本: [138.2529, 36.2048],
  韩国: [127.7669, 35.9078],
  新加坡: [103.8198, 1.3521],
  德国: [10.4515, 51.1657],
  英国: [-3.436, 55.3781],
  法国: [2.2137, 46.2276],
  加拿大: [-106.3468, 56.1304],
  澳大利亚: [133.7751, -25.2744],
  俄罗斯: [105.3188, 61.524],
  荷兰: [5.2913, 52.1326],
  印度: [78.9629, 20.5937],
  巴西: [-51.9253, -14.235],
  爱尔兰: [-8.2439, 53.4129],
  越南: [108.2772, 14.0583],
  泰国: [100.9925, 15.87],
  菲律宾: [121.774, 12.8797],
  印度尼西亚: [113.9213, -0.7893],
  马来西亚: [101.9758, 4.2105],
  阿联酋: [53.8478, 23.4241],
  南非: [22.9375, -30.5595],
  土耳其: [35.2433, 38.9637],
  意大利: [12.5674, 41.8719],
  西班牙: [-3.7492, 40.4637],
  瑞典: [18.6435, 60.1282],
  瑞士: [8.2275, 46.8182],
  波兰: [19.1451, 51.9194],
  中国: [104.1954, 35.8617],
  香港: [114.1694, 22.3193],
  台湾: [121.0, 23.7],
}

function loadMapLibre(): Promise<MapLibreGlobal> {
  if (window.maplibregl) {
    return Promise.resolve(window.maplibregl)
  }

  const cssId = 'maplibre-gl-css'
  if (!document.getElementById(cssId)) {
    const link = document.createElement('link')
    link.id = cssId
    link.rel = 'stylesheet'
    link.href = mapLibreCSSURL
    document.head.appendChild(link)
  }

  return new Promise((resolve, reject) => {
    let settled = false
    const timeout = window.setTimeout(() => {
      finish(() => reject(new Error('MapLibre script load timed out')))
    }, mapLibreLoadTimeoutMS)

    const finish = (callback: () => void) => {
      if (settled) return
      settled = true
      window.clearTimeout(timeout)
      callback()
    }

    const resolveGlobal = () => {
      if (window.maplibregl) {
        finish(() => resolve(window.maplibregl as MapLibreGlobal))
      } else {
        finish(() => reject(new Error('MapLibre global is not available')))
      }
    }
    const rejectLoad = () => finish(() => reject(new Error('MapLibre script failed to load')))

    const existingScript = document.getElementById('maplibre-gl-js') as HTMLScriptElement | null
    if (existingScript) {
      existingScript.addEventListener('load', resolveGlobal, { once: true })
      existingScript.addEventListener('error', rejectLoad, { once: true })
      return
    }

    const script = document.createElement('script')
    script.id = 'maplibre-gl-js'
    script.src = mapLibreScriptURL
    script.async = true
    script.addEventListener('load', resolveGlobal, { once: true })
    script.addEventListener('error', rejectLoad, { once: true })
    document.head.appendChild(script)
  })
}

let geoPromise: Promise<{ china: GeoCollection; world: GeoCollection }> | null = null

function loadGeoData() {
  if (!geoPromise) {
    geoPromise = Promise.all([
      fetch(chinaGeoURL, { cache: 'force-cache' }).then((response) => {
        if (!response.ok) throw new Error(`china geo HTTP ${response.status}`)
        return response.json() as Promise<GeoCollection>
      }),
      fetch(worldGeoURL, { cache: 'force-cache' }).then((response) => {
        if (!response.ok) throw new Error(`world geo HTTP ${response.status}`)
        return response.json() as Promise<GeoCollection>
      }),
    ]).then(([china, world]) => ({ china, world }))
  }
  return geoPromise
}

function shortProvinceName(name: string) {
  return name
    .replace(/特别行政区$/, '')
    .replace(/维吾尔自治区$/, '')
    .replace(/壮族自治区$/, '')
    .replace(/回族自治区$/, '')
    .replace(/自治区$/, '')
    .replace(/省$/, '')
    .replace(/市$/, '')
}

function normalizeCityName(value: string | undefined) {
  if (!value) return ''
  return value.replace(/(特别行政区|自治州|地区|市)$/, '')
}

function escapeHTML(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function formatNumber(value: number, isEnglish: boolean) {
  return Math.max(0, value).toLocaleString(isEnglish ? 'en-US' : 'zh-CN')
}

function formatTime(value: string | undefined, isEnglish: boolean) {
  if (!value) return isEnglish ? '—' : '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString(isEnglish ? 'en-US' : 'zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

const defaultPalette: Palette = {
  scale: ['#141821', '#182138', '#1f2f52', '#2a4677', '#4a78e8'],
  landLine: '#3f3f46',
  worldFill: '#27272a',
  worldLine: '#3f3f46',
  city: '#a9c6ff',
  citySelected: '#ffffff',
  selectedFill: 'rgba(120, 162, 255, 0.16)',
  selectedLine: '#e4e4e7',
  labelStrong: '#eef3ff',
  dotStroke: '#09090b',
}

function readPalette(host: HTMLElement | null): Palette {
  if (typeof window === 'undefined') return defaultPalette
  const style = host ? window.getComputedStyle(host) : window.getComputedStyle(document.documentElement)
  const read = (name: string, fallback: string) => style.getPropertyValue(name).trim() || fallback
  return {
    scale: [
      read('--asset-map-scale-1', '#141821'),
      read('--asset-map-scale-2', '#182138'),
      read('--asset-map-scale-3', '#1f2f52'),
      read('--asset-map-scale-4', '#2a4677'),
      read('--asset-map-scale-5', '#4a78e8'),
    ],
    landLine: read('--asset-map-region-line', '#3f3f46'),
    worldFill: read('--asset-map-world-fill', '#27272a'),
    worldLine: read('--asset-map-world-line', '#3f3f46'),
    city: read('--asset-map-city', '#a9c6ff'),
    citySelected: read('--asset-map-city-selected', '#ffffff'),
    selectedFill: 'rgba(120, 162, 255, 0.16)',
    selectedLine: read('--asset-map-region-selected-line', '#e4e4e7'),
    labelStrong: read('--asset-map-label-strong', '#eef3ff'),
    dotStroke: read('--asset-map-point-stroke', '#09090b'),
  }
}

function buildChoroplethExpression(palette: Palette, max: number, colorMode: MapColorMode) {
  const property = colorMode === 'risk' ? 'risk' : 'count'
  const ceiling = Math.max(1, max)
  return [
    'interpolate',
    ['linear'],
    ['get', property],
    0,
    palette.scale[0],
    ceiling * 0.06,
    palette.scale[1],
    ceiling * 0.22,
    palette.scale[2],
    ceiling * 0.52,
    palette.scale[3],
    ceiling,
    palette.scale[4],
  ]
}

function cityRadiusExpression(max: number) {
  const ceiling = Math.max(1, max)
  return [
    'interpolate',
    ['linear'],
    ['get', 'count'],
    0,
    2.2,
    ceiling * 0.15,
    3.4,
    ceiling * 0.45,
    5.2,
    ceiling,
    8.4,
  ]
}

function hashString(value: string) {
  let hash = 0
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0
  }
  return hash
}

function cityFeatureCoordinate(
  province: string,
  city: string,
  provinceCenters: Map<string, [number, number]>
): [number, number] | null {
  const direct = CITY_COORDINATES[city]
  if (direct) return [direct[0], direct[1]]
  const center = provinceCenters.get(province)
  if (!center) return null
  const hash = hashString(`${province}|${city}`)
  const angle = (hash % 360) * (Math.PI / 180)
  const radius = 0.3 + ((hash >> 9) % 40) / 100
  return [center[0] + Math.cos(angle) * radius, center[1] + Math.sin(angle) * radius * 0.72]
}

/* -------------------------------------------------------------------------- */
/* Component                                                                  */
/* -------------------------------------------------------------------------- */

export function AssetMapView() {
  const { language } = useI18n()
  const isEnglish = language === 'en'

  const copy = isEnglish
    ? {
        title: 'Asset Map',
        subtitle: 'Public IP distribution',
        refresh: 'Refresh',
        updatedAt: 'Updated',
        located: 'Located',
        internal: 'Internal',
        unresolved: 'Unresolved',
        overseas: 'Overseas',
        domestic: 'China',
        world: 'World',
        searchPlaceholder: 'Search IP / CIDR / operator',
        windowAll: 'All time',
        window7d: 'Last 7 days',
        window30d: 'Last 30 days',
        window90d: 'Last 90 days',
        provinces: 'Provinces',
        countries: 'Countries',
        regionCount: 'regions',
        assets: 'assets',
        noData: 'No located public IP assets yet',
        noDataHint: 'Start a scan or import assets to populate the map.',
        internalOnly: 'Internal addresses are excluded from the geographic view.',
        unresolvedOnly: 'Public IPs without a location cannot be placed on the map.',
        overseasOnly: 'Overseas public IP assets by country.',
        mapLoadFailed: 'Failed to initialize the map',
        geoLoadFailed: 'Offline boundary data failed to load',
        loadFailed: 'Failed to load asset map',
        assetsLoadFailed: 'Failed to load assets',
        legendTitle: 'Provincial assets',
        legendRisk: 'Critical + high (incl. inherited)',
        colorBy: 'Color',
        colorByAssets: 'Assets',
        colorByRisk: 'Risk',
        selected: 'Selected',
        tabAssets: 'Assets',
        tabCities: 'Cities',
        tabOperators: 'Operators / ASN',
        all: 'All',
        critical: 'Critical',
        high: 'High',
        medium: 'Medium',
        risky: 'At risk',
        sortRisk: 'By risk',
        cityDistribution: 'City distribution',
        operatorDistribution: 'Operators',
        asnDistribution: 'ASN',
        noStats: 'No statistics',
        noAssets: 'No matching assets',
        noRiskData: 'No critical/high risk data yet',
        inherited: 'inherited',
        directRisk: 'Direct',
        inheritedRisk: 'Inherited',
        unknownCity: 'Unknown city',
        loadMore: 'Load more',
        shown: 'shown',
        ledger: 'Asset ledger',
        detailHint: 'Open asset details',
        risk: 'Risk',
        ports: 'Ports',
        clearCity: 'Clear city filter',
        close: 'Close',
        legacyAsset: 'This legacy asset is not in the unified ledger yet',
        totalRisk: 'Critical / High',
      }
    : {
        title: '资产地图',
        subtitle: '公网 IP 地理分布',
        refresh: '刷新',
        updatedAt: '更新于',
        located: '已定位',
        internal: '内网',
        unresolved: '未定位',
        overseas: '境外',
        domestic: '国内',
        world: '海外',
        searchPlaceholder: '搜索 IP / C 段 / 运营商',
        windowAll: '全部时间',
        window7d: '近 7 天',
        window30d: '近 30 天',
        window90d: '近 90 天',
        provinces: '省份排行',
        countries: '国家排行',
        regionCount: '个区域',
        assets: '资产',
        noData: '暂无已定位公网 IP 资产',
        noDataHint: '执行扫描或导入资产后，这里会显示地理分布。',
        internalOnly: '内网地址不参与地理分布，右侧列出明细。',
        unresolvedOnly: '公网 IP 尚未解析出地理位置，无法落到地图。',
        overseasOnly: '境外公网 IP 按国家分布。',
        mapLoadFailed: '地图初始化失败',
        geoLoadFailed: '离线边界数据加载失败',
        loadFailed: '加载资产地图失败',
        assetsLoadFailed: '加载资产列表失败',
        legendTitle: '省级资产数',
        legendRisk: '省级严重+高危（含继承）',
        colorBy: '着色',
        colorByAssets: '资产数',
        colorByRisk: '高危数',
        selected: '已选',
        tabAssets: '资产列表',
        tabCities: '城市分布',
        tabOperators: '运营商 / ASN',
        all: '全部',
        critical: '严重',
        high: '高危',
        medium: '中危',
        risky: '有风险',
        sortRisk: '按风险',
        cityDistribution: '城市分布',
        operatorDistribution: '运营商',
        asnDistribution: 'ASN',
        noStats: '暂无统计',
        noAssets: '没有匹配的资产',
        noRiskData: '暂无严重/高危风险数据',
        inherited: '继承',
        directRisk: '直接',
        inheritedRisk: '继承',
        unknownCity: '未知城市',
        loadMore: '加载更多',
        shown: '已显示',
        ledger: '资产台账',
        detailHint: '查看资产详情',
        risk: '风险',
        ports: '端口',
        clearCity: '清除城市筛选',
        close: '关闭',
        legacyAsset: '该资产来自历史扫描记录，尚未进入统一台账',
        totalRisk: '严重 / 高危',
      }

  const mapContainerRef = useRef<HTMLDivElement | null>(null)
  const mapRef = useRef<MapLibreMap | null>(null)
  const maplibreRef = useRef<MapLibreGlobal | null>(null)
  const popupRef = useRef<MapLibrePopup | null>(null)
  const cityClickAtRef = useRef(0)
  const markersRef = useRef<MapLibreMarker[]>([])
  const chinaGeoRef = useRef<GeoCollection | null>(null)
  const worldGeoRef = useRef<GeoCollection | null>(null)
  const provinceCentersRef = useRef<Map<string, [number, number]>>(new Map())
  const provinceFeaturesRef = useRef<Map<string, GeoFeature>>(new Map())


  const [mapReady, setMapReady] = useState(false)
  const [geoVersion, setGeoVersion] = useState(0)
  const [palette, setPalette] = useState<Palette>(defaultPalette)
  const [overview, setOverview] = useState<AssetMapOverview | null>(null)
  const [overviewLoading, setOverviewLoading] = useState(true)
  const [assets, setAssets] = useState<AssetMapAsset[]>([])
  const [assetsTotal, setAssetsTotal] = useState(0)
  const [assetsLoading, setAssetsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [mapError, setMapError] = useState<string | null>(null)

  const [mode, setMode] = useState<MapMode>('china')
  const [scope, setScope] = useState<MapScope>('located')
  const [selectedProvince, setSelectedProvince] = useState('')
  const [selectedCity, setSelectedCity] = useState('')
  const [selectedCountry, setSelectedCountry] = useState('')
  const [colorMode, setColorMode] = useState<MapColorMode>('assets')
  const [risk, setRisk] = useState<RiskFilter>('all')
  const [windowKey, setWindowKey] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchTerm, setSearchTerm] = useState('')
  const [detailTab, setDetailTab] = useState<DetailTab>('assets')
  const [page, setPage] = useState(1)
  const [refreshKey, setRefreshKey] = useState(0)
  const [detailAssetId, setDetailAssetId] = useState<string | null>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearchTerm(searchInput.trim())
      setPage(1)
    }, 320)
    return () => window.clearTimeout(timer)
  }, [searchInput])

  const selectProvinceRef = useRef<((province: string) => void) | null>(null)
  const selectCityRef = useRef<((province: string, city: string) => void) | null>(null)
  const selectCountryRef = useRef<((country: string) => void) | null>(null)

  const selectedRegion = useMemo(
    () => overview?.regions.find((region) => region.province === selectedProvince) || null,
    [overview, selectedProvince]
  )
  const selectedCountryStat = useMemo(
    () => overview?.countries.find((country) => (country.country || country.name) === selectedCountry) || null,
    [overview, selectedCountry]
  )

  const maxRegionCount = useMemo(
    () => Math.max(1, ...(overview?.regions || []).map((region) => region.assetCount)),
    [overview]
  )
  const maxCountryCount = useMemo(
    () => Math.max(1, ...(overview?.countries || []).map((country) => country.assetCount || country.count || 0)),
    [overview]
  )

  const scopeItems = useMemo(
    () => [
      { key: 'located' as const, label: copy.domestic, value: overview?.locatedIpAssets || 0, icon: Crosshair },
      { key: 'overseas' as const, label: copy.overseas, value: overview?.overseasIpAssets || 0, icon: Globe2 },
    ],
    [copy.domestic, copy.overseas, overview]
  )

  const totalRiskCount = (overview?.totalCritical || 0) + (overview?.totalHigh || 0)
  const hasRiskData = totalRiskCount > 0

  const isWorldMode = mode === 'world'
  const isGeoScope = scope === 'located' || scope === 'overseas'
  const rankingDisabled = scope === 'internal' || scope === 'unresolved'
  const rankItems = isWorldMode ? overview?.countries || [] : overview?.regions || []
  const rankMax = isWorldMode ? maxCountryCount : maxRegionCount

  const detailRegion = isWorldMode ? null : selectedRegion
  const detailCountry = isWorldMode ? selectedCountryStat : null
  const detailAssetCount = rankingDisabled
    ? scope === 'internal'
      ? overview?.nonPublicIpAssets || 0
      : overview?.unresolvedIpAssets || 0
    : isWorldMode
    ? detailCountry?.assetCount || detailCountry?.count || 0
    : detailRegion?.assetCount || 0
  const detailCritical = isWorldMode ? detailCountry?.criticalCount || 0 : detailRegion?.criticalCount || 0
  const detailHigh = isWorldMode ? detailCountry?.highCount || 0 : detailRegion?.highCount || 0
  const detailMedium = isWorldMode ? detailCountry?.mediumCount || 0 : detailRegion?.mediumCount || 0
  const detailInherited =
    (isWorldMode ? detailCountry?.inheritedCritical || 0 : detailRegion?.inheritedCritical || 0) +
    (isWorldMode ? detailCountry?.inheritedHigh || 0 : detailRegion?.inheritedHigh || 0) +
    (isWorldMode ? detailCountry?.inheritedMedium || 0 : detailRegion?.inheritedMedium || 0)
  const detailCities = (isWorldMode ? detailCountry?.cities || [] : detailRegion?.cities || []).filter(
    (city) =>
      isWorldMode ||
      !detailRegion ||
      municipalityProvinces.has(detailRegion.province) ||
      (city.name !== detailRegion.province && city.name !== detailRegion.provinceLabel)
  )
  const detailOperators = isWorldMode ? detailCountry?.operators || [] : detailRegion?.operators || []
  const detailAsns = isWorldMode ? detailCountry?.asns || [] : detailRegion?.asns || []
  const detailTitle = rankingDisabled
    ? scope === 'internal'
      ? copy.internal
      : copy.unresolved
    : isWorldMode
    ? detailCountry?.countryLabel || detailCountry?.name || copy.overseas
    : detailRegion?.provinceLabel || copy.domestic
  const detailSubtitle = rankingDisabled
    ? scope === 'internal'
      ? copy.internalOnly
      : copy.unresolvedOnly
    : isWorldMode
    ? copy.overseasOnly
    : selectedCity
    ? `${detailRegion?.provinceLabel || ''} · ${selectedCity}`
    : detailRegion
    ? detailRegion.cities.slice(0, 3).map((city) => `${city.name} ${formatNumber(city.count, isEnglish)}`).join(' · ')
    : copy.noData

  const showEmptyMap =
    scope === 'located' && !overviewLoading && (overview?.locatedIpAssets || 0) === 0 && !overview?.regions.length

  /* ------------------------------- data ---------------------------------- */

  useEffect(() => {
    const controller = new AbortController()
    setOverviewLoading(true)
    setError(null)
    const params = new URLSearchParams()
    if (windowKey) params.set('window', windowKey)
    fetchWithAuth(`/api/v1/assets/map/overview?${params.toString()}`, {
      cache: 'no-store',
      signal: controller.signal,
    })
      .then(async (response) => {
        const payload = (await response.json()) as {
          success?: boolean
          data?: AssetMapOverview
          error?: string
        }
        if (!response.ok || !payload.success || !payload.data) {
          throw new Error(payload.error || copy.loadFailed)
        }
        setOverview(payload.data)
      })
      .catch((requestError) => {
        if (controller.signal.aborted) return
        const message = requestError instanceof Error ? requestError.message : copy.loadFailed
        setError(message)
        toast.error(message)
      })
      .finally(() => {
        if (!controller.signal.aborted) setOverviewLoading(false)
      })
    return () => controller.abort()
  }, [copy.loadFailed, refreshKey, windowKey])

  useEffect(() => {
    if (colorMode === 'risk' && !hasRiskData) setColorMode('assets')
  }, [colorMode, hasRiskData])

  useEffect(() => {
    if (!overview) return
    if (mode === 'china') {
      if (scope === 'located') {
        setSelectedProvince((current) => {
          if (current && overview.regions.some((region) => region.province === current)) return current
          return overview.regions[0]?.province || ''
        })
      }
    } else if (scope === 'overseas') {
      setSelectedCountry((current) => {
        if (current && overview.countries.some((country) => (country.country || country.name) === current)) return current
        return overview.countries[0]?.country || overview.countries[0]?.name || ''
      })
    }
  }, [mode, overview, scope])

  useEffect(() => {
    const controller = new AbortController()
    setAssetsLoading(true)
    const params = new URLSearchParams({
      scope,
      page: String(page),
      pageSize: '50',
    })
    if (scope === 'located' && selectedProvince) params.set('province', selectedProvince)
    if (scope === 'located' && selectedCity) params.set('city', selectedCity)
    if (scope === 'overseas' && selectedCountry) params.set('country', selectedCountry)
    if (risk !== 'all') params.set('risk', risk)
    if (searchTerm) params.set('keyword', searchTerm)
    if (windowKey) params.set('window', windowKey)
    fetchWithAuth(`/api/v1/assets/map/assets?${params.toString()}`, {
      cache: 'no-store',
      signal: controller.signal,
    })
      .then(async (response) => {
        const payload = (await response.json()) as {
          success?: boolean
          data?: AssetMapAssetList
          error?: string
        }
        if (!response.ok || !payload.success || !payload.data) {
          throw new Error(payload.error || copy.assetsLoadFailed)
        }
        const data = payload.data
        setAssets((current) => (page === 1 ? data.items : [...current, ...data.items]))
        setAssetsTotal(data.total)
      })
      .catch((requestError) => {
        if (controller.signal.aborted) return
        toast.error(requestError instanceof Error ? requestError.message : copy.assetsLoadFailed)
      })
      .finally(() => {
        if (!controller.signal.aborted) setAssetsLoading(false)
      })
    return () => controller.abort()
  }, [copy.assetsLoadFailed, page, refreshKey, risk, scope, searchTerm, selectedCity, selectedCountry, selectedProvince, windowKey])

  /* ------------------------------- map ----------------------------------- */

  useEffect(() => {
    let disposed = false
    let readyTimer: number | null = null
    let resizeObserver: ResizeObserver | null = null
    const container = mapContainerRef.current
    if (!container) return

    const initialize = async () => {
      try {
        const [maplibregl, geo] = await Promise.all([loadMapLibre(), loadGeoData()])
        if (disposed) return

        // The page can mount inside a tab panel that has not received its
        // final size yet. Creating MapLibre against a 0x0 container leaves a
        // 0x0 canvas that never draws, so wait for a real layout first.
        for (let attempt = 0; attempt < 30; attempt += 1) {
          if (container.clientWidth > 0 && container.clientHeight > 0) break
          await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()))
          if (disposed) return
        }
        maplibreRef.current = maplibregl
        chinaGeoRef.current = geo.china
        worldGeoRef.current = geo.world

        const centers = new Map<string, [number, number]>()
        const features = new Map<string, GeoFeature>()
        for (const feature of geo.china.features) {
          const name = String(feature.properties.name || '')
          const center = feature.properties.center as number[] | undefined
          if (!name || !center) continue
          const province = shortProvinceName(name)
          centers.set(province, [center[0], center[1]])
          features.set(province, feature)
        }
        provinceCentersRef.current = centers
        provinceFeaturesRef.current = features

        if (maplibregl.setWorkerUrl) {
          maplibregl.setWorkerUrl(mapLibreWorkerURL)
        }
        const map = new maplibregl.Map({
          container,
          style: blankMapStyle,
          center: chinaCenter,
          zoom: 3.2,
          minZoom: 0.8,
          maxZoom: 7,
          attributionControl: false,
        })
        map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
        mapRef.current = map
        resizeObserver = new ResizeObserver(() => map.resize?.())
        resizeObserver.observe(container)
        window.requestAnimationFrame(() => map.resize?.())
        window.setTimeout(() => map.resize?.(), 200)
        window.setTimeout(() => map.resize?.(), 600)

        map.on('error', (event) => {
          const message = (event as { error?: Error }).error?.message
          if (message) {
            console.error('[asset-map]', message)
            setMapError(message)
          }
        })

        window.setTimeout(() => {
          if (disposed) return
          const canvas = map.getCanvas()
          if (!canvas.width || !canvas.height) {
            console.warn(`[asset-map] canvas has no size: ${canvas.width}x${canvas.height}`)
          }
        }, 2500)

        readyTimer = window.setTimeout(() => {
          if (!disposed) setMapError(copy.mapLoadFailed)
        }, mapLibreLoadTimeoutMS)

        map.on('load', () => {
          map.resize?.()
          if (readyTimer !== null) {
            window.clearTimeout(readyTimer)
            readyTimer = null
          }
          if (disposed) return
          const currentPalette = readPalette(container)
          setPalette(currentPalette)

          map.addSource('map-world', { type: 'geojson', data: geo.world })
          map.addLayer({
            id: 'map-world-fill',
            type: 'fill',
            source: 'map-world',
            paint: {
              'fill-color': currentPalette.worldFill,
              'fill-opacity': 0.25,
            },
          })
          map.addLayer({
            id: 'map-world-line',
            type: 'line',
            source: 'map-world',
            paint: {
              'line-color': currentPalette.worldLine,
              'line-width': 0.5,
              'line-opacity': 0.5,
            },
          })

          map.addSource('map-provinces', { type: 'geojson', data: geo.china })
          map.addLayer({
            id: 'map-province-fill',
            type: 'fill',
            source: 'map-provinces',
            paint: {
              'fill-color': buildChoroplethExpression(currentPalette, 1, 'assets'),
              'fill-opacity': 1,
            },
          })
          map.addLayer({
            id: 'map-province-selected-fill',
            type: 'fill',
            source: 'map-provinces',
            filter: ['==', ['get', 'province'], ''],
            paint: {
              'fill-color': currentPalette.selectedFill,
              'fill-opacity': 1,
            },
          })
          map.addLayer({
            id: 'map-province-line',
            type: 'line',
            source: 'map-provinces',
            paint: {
              'line-color': currentPalette.landLine,
              'line-width': 0.7,
              'line-opacity': 0.9,
            },
          })
          map.addLayer({
            id: 'map-province-selected-line',
            type: 'line',
            source: 'map-provinces',
            filter: ['==', ['get', 'province'], ''],
            paint: {
              'line-color': currentPalette.selectedLine,
              'line-width': 1.7,
              'line-opacity': 1,
            },
          })

          map.addSource('map-cities', {
            type: 'geojson',
            data: { type: 'FeatureCollection', features: [] },
          })
          map.addLayer({
            id: 'map-city-halo',
            type: 'circle',
            source: 'map-cities',
            paint: {
              'circle-color': currentPalette.city,
              'circle-opacity': 0.14,
              'circle-radius': [
                'interpolate',
                ['linear'],
                ['get', 'count'],
                0,
                4,
                1,
                6,
                50,
                14,
                400,
                26,
              ],
            },
          })
          map.addLayer({
            id: 'map-city-dot',
            type: 'circle',
            source: 'map-cities',
            paint: {
              'circle-color': ['case', ['==', ['get', 'selected'], 1], currentPalette.citySelected, currentPalette.city],
              'circle-stroke-color': currentPalette.dotStroke,
              'circle-stroke-width': ['case', ['==', ['get', 'selected'], 1], 1.6, 1],
              'circle-radius': cityRadiusExpression(1),
              'circle-opacity': 0.95,
            },
          })
          map.addLayer({
            id: 'map-city-ring',
            type: 'circle',
            source: 'map-cities',
            filter: ['==', ['get', 'selected'], 1],
            paint: {
              'circle-color': 'rgba(0,0,0,0)',
              'circle-stroke-color': currentPalette.citySelected,
              'circle-stroke-width': 1,
              'circle-stroke-opacity': 0.55,
              'circle-radius': ['+', cityRadiusExpression(1), 7],
            },
          })

          map.addSource('map-country-points', {
            type: 'geojson',
            data: { type: 'FeatureCollection', features: [] },
          })
          map.addLayer({
            id: 'map-country-halo',
            type: 'circle',
            source: 'map-country-points',
            paint: {
              'circle-color': currentPalette.city,
              'circle-opacity': 0.16,
              'circle-radius': ['interpolate', ['linear'], ['get', 'count'], 0, 6, 1, 9, 100, 22],
            },
          })
          map.addLayer({
            id: 'map-country-dot',
            type: 'circle',
            source: 'map-country-points',
            paint: {
              'circle-color': ['case', ['==', ['get', 'selected'], 1], currentPalette.citySelected, currentPalette.city],
              'circle-stroke-color': currentPalette.dotStroke,
              'circle-stroke-width': ['case', ['==', ['get', 'selected'], 1], 2, 1.2],
              'circle-radius': ['interpolate', ['linear'], ['get', 'count'], 0, 3, 1, 4, 100, 8],
              'circle-opacity': 0.95,
            },
          })

          popupRef.current = new maplibregl.Popup({
            closeButton: false,
            closeOnClick: false,
            className: 'asset-map-hover-popup',
            offset: 12,
          })

          const showTooltip = (
            event: Record<string, unknown>,
            title: string,
            rows: Array<[string, string]>,
            hint: string
          ) => {
            const popup = popupRef.current
            if (!popup) return
            const rowsHTML = rows
              .map(([label, value]) => `<span>${escapeHTML(label)} <b>${escapeHTML(value)}</b></span>`)
              .join('')
            popup
              .setLngLat((event.lngLat as [number, number]) || chinaCenter)
              .setHTML(
                `<div class="asset-map-tooltip"><div class="asset-map-tooltip-title">${escapeHTML(title)}</div>` +
                  `<div class="asset-map-tooltip-rows">${rowsHTML}</div>` +
                  `<div class="asset-map-tooltip-hint">${escapeHTML(hint)}</div></div>`
              )
              .addTo(map)
          }

          map.on('mousemove', 'map-province-fill', (event) => {
            const feature = (event.features as Array<{ properties?: Record<string, unknown> }> | undefined)?.[0]
            if (!feature?.properties) return
            const props = feature.properties
            const count = Number(props.count || 0)
            const riskCount = Number(props.critical || 0) + Number(props.high || 0)
            const inherited = Number(props.inherited || 0)
            const medium = Number(props.medium || 0)
            const rows: Array<[string, string]> = [
              [copy.assets, formatNumber(count, isEnglish)],
              [copy.totalRisk, formatNumber(riskCount, isEnglish)],
            ]
            if (medium > 0) rows.push([copy.medium, formatNumber(medium, isEnglish)])
            if (inherited > 0) rows.push([copy.inherited, formatNumber(inherited, isEnglish)])
            showTooltip(event, String(props.label || props.province || ''), rows, copy.detailHint)
            map.getCanvas().style.cursor = 'pointer'
          })
          map.on('mouseleave', 'map-province-fill', () => {
            popupRef.current?.remove()
            map.getCanvas().style.cursor = ''
          })
          map.on('click', 'map-province-fill', (event) => {
            if (Date.now() - cityClickAtRef.current < 200) return
            const feature = (event.features as Array<{ properties?: Record<string, unknown> }> | undefined)?.[0]
            const province = String(feature?.properties?.province || '')
            if (province) selectProvinceRef.current?.(province)
          })

          map.on('mousemove', 'map-city-dot', (event) => {
            const feature = (event.features as Array<{ properties?: Record<string, unknown> }> | undefined)?.[0]
            if (!feature?.properties) return
            const props = feature.properties
            showTooltip(
              event,
              `${String(props.city || '')} · ${String(props.provinceLabel || '')}`,
              [[copy.assets, formatNumber(Number(props.count || 0), isEnglish)]],
              copy.detailHint
            )
            map.getCanvas().style.cursor = 'pointer'
          })
          map.on('mouseleave', 'map-city-dot', () => {
            popupRef.current?.remove()
            map.getCanvas().style.cursor = ''
          })
          map.on('click', 'map-city-dot', (event) => {
            cityClickAtRef.current = Date.now()
            const feature = (event.features as Array<{ properties?: Record<string, unknown> }> | undefined)?.[0]
            if (!feature?.properties) return
            selectCityRef.current?.(String(feature.properties.province || ''), String(feature.properties.city || ''))
          })

          map.on('mousemove', 'map-country-dot', (event) => {
            const feature = (event.features as Array<{ properties?: Record<string, unknown> }> | undefined)?.[0]
            if (!feature?.properties) return
            const props = feature.properties
            const rows: Array<[string, string]> = [
              [copy.assets, formatNumber(Number(props.count || 0), isEnglish)],
              [copy.totalRisk, formatNumber(Number(props.risk || 0), isEnglish)],
            ]
            if (Number(props.medium || 0) > 0) {
              rows.push([copy.medium, formatNumber(Number(props.medium), isEnglish)])
            }
            if (Number(props.inherited || 0) > 0) {
              rows.push([copy.inherited, formatNumber(Number(props.inherited), isEnglish)])
            }
            showTooltip(event, String(props.label || ''), rows, copy.detailHint)
            map.getCanvas().style.cursor = 'pointer'
          })
          map.on('mouseleave', 'map-country-dot', () => {
            popupRef.current?.remove()
            map.getCanvas().style.cursor = ''
          })
          map.on('click', 'map-country-dot', (event) => {
            const feature = (event.features as Array<{ properties?: Record<string, unknown> }> | undefined)?.[0]
            const country = String(feature?.properties?.country || '')
            if (country) selectCountryRef.current?.(country)
          })

          setGeoVersion((version) => version + 1)
          setMapReady(true)
        })
      } catch (mapInitError) {
        if (disposed) return
        setMapError(mapInitError instanceof Error ? mapInitError.message : copy.mapLoadFailed)
        setOverviewLoading(false)
      }
    }

    void initialize()
    return () => {
      disposed = true
      if (readyTimer !== null) window.clearTimeout(readyTimer)
      resizeObserver?.disconnect()
      popupRef.current?.remove()
      markersRef.current.forEach((marker) => marker.remove())
      markersRef.current = []
      mapRef.current?.remove()
      mapRef.current = null
    }
    // The map is initialized once; data updates are handled by the effects below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!mapReady) return
    const map = mapRef.current
    if (!map) return
    const update = () => {
      setPalette(readPalette(mapContainerRef.current))
    }
    const observer = new MutationObserver(update)
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class', 'data-theme'] })
    return () => observer.disconnect()
  }, [mapReady])

  // Keep province risk/counters, city points and country points in sync.
  useEffect(() => {
    const map = mapRef.current
    const china = chinaGeoRef.current
    const world = worldGeoRef.current
    if (!mapReady || !map || !china || !world || !overview) return

    const regionsByProvince = new Map(overview.regions.map((region) => [region.province, region]))
    const maxCount = Math.max(1, ...overview.regions.map((region) => (colorMode === 'risk' ? region.criticalCount + region.highCount : region.assetCount)))
    const decoratedProvinces: GeoCollection = {
      type: 'FeatureCollection',
      features: china.features.map((feature) => {
        const name = String(feature.properties.name || '')
        const province = shortProvinceName(name)
        const region = regionsByProvince.get(province)
        return {
          ...feature,
          properties: {
            ...feature.properties,
            province,
            label: region?.provinceLabel || name,
            count: region?.assetCount || 0,
            risk: (region?.criticalCount || 0) + (region?.highCount || 0),
            inherited: (region?.inheritedCritical || 0) + (region?.inheritedHigh || 0) + (region?.inheritedMedium || 0),
            medium: region?.mediumCount || 0,
            critical: region?.criticalCount || 0,
            high: region?.highCount || 0,
          },
        }
      }),
    }
    map.getSource('map-provinces')?.setData?.(decoratedProvinces)
    map.setPaintProperty('map-province-fill', 'fill-color', buildChoroplethExpression(palette, maxCount, colorMode))
    map.setFilter('map-province-selected-fill', ['==', ['get', 'province'], selectedProvince])
    map.setFilter('map-province-selected-line', ['==', ['get', 'province'], selectedProvince])

    const centers = provinceCentersRef.current
    const cityFeatures: Array<Record<string, unknown>> = []
    let maxCityCount = 1
    for (const region of overview.regions) {
      for (const city of region.cities) {
        if (!municipalityProvinces.has(region.province) && (city.name === region.province || city.name === region.provinceLabel)) continue
        maxCityCount = Math.max(maxCityCount, city.count)
        const coordinate = cityFeatureCoordinate(region.province, city.name, centers)
        if (!coordinate) continue
        cityFeatures.push({
          type: 'Feature',
          properties: {
            province: region.province,
            provinceLabel: region.provinceLabel,
            city: city.name,
            count: city.count,
            selected: region.province === selectedProvince && city.name === selectedCity ? 1 : 0,
          },
          geometry: { type: 'Point', coordinates: coordinate },
        })
      }
    }
    map.getSource('map-cities')?.setData?.({ type: 'FeatureCollection', features: cityFeatures })
    map.setPaintProperty('map-city-dot', 'circle-radius', cityRadiusExpression(maxCityCount))
    map.setPaintProperty('map-city-halo', 'circle-color', palette.city)
    map.setPaintProperty('map-city-dot', 'circle-color', ['case', ['==', ['get', 'selected'], 1], palette.citySelected, palette.city])
    map.setPaintProperty('map-city-dot', 'circle-stroke-color', palette.dotStroke)
    map.setPaintProperty('map-city-ring', 'circle-stroke-color', palette.citySelected)
    map.setPaintProperty('map-city-ring', 'circle-radius', ['+', cityRadiusExpression(maxCityCount), 7])
    map.setPaintProperty('map-world-fill', 'fill-color', palette.worldFill)
    map.setPaintProperty('map-world-line', 'line-color', palette.worldLine)

    const countryFeatures: Array<Record<string, unknown>> = []
    for (const country of overview.countries) {
      const key = country.country || country.name
      const coordinate = countryCoordinates[key]
      if (!coordinate) continue
      countryFeatures.push({
        type: 'Feature',
        properties: {
          country: key,
          label: country.countryLabel || country.name,
          count: country.assetCount || country.count || 0,
          risk: (country.criticalCount || 0) + (country.highCount || 0),
          inherited: (country.inheritedCritical || 0) + (country.inheritedHigh || 0) + (country.inheritedMedium || 0),
          medium: country.mediumCount || 0,
          selected: key === selectedCountry ? 1 : 0,
        },
        geometry: { type: 'Point', coordinates: coordinate },
      })
    }
    map.getSource('map-country-points')?.setData?.({ type: 'FeatureCollection', features: countryFeatures })
    map.setPaintProperty('map-country-halo', 'circle-color', palette.city)
    map.setPaintProperty('map-country-dot', 'circle-color', ['case', ['==', ['get', 'selected'], 1], palette.citySelected, palette.city])
    map.setPaintProperty('map-country-dot', 'circle-stroke-color', palette.dotStroke)
  }, [colorMode, mapReady, overview, palette, selectedCountry, selectedCity, selectedProvince])

  // Mode switching (China provinces vs world countries).
  useEffect(() => {
    const map = mapRef.current
    if (!mapReady || !map) return
    const chinaLayers = ['map-province-fill', 'map-province-selected-fill', 'map-province-line', 'map-province-selected-line', 'map-city-halo', 'map-city-dot', 'map-city-ring']
    const worldLayers = ['map-country-halo', 'map-country-dot']
    const chinaVisible = !isWorldMode
    for (const layer of chinaLayers) {
      map.setLayoutProperty(layer, 'visibility', chinaVisible ? 'visible' : 'none')
    }
    for (const layer of worldLayers) {
      map.setLayoutProperty(layer, 'visibility', chinaVisible ? 'none' : 'visible')
    }
    map.setPaintProperty('map-world-fill', 'fill-opacity', chinaVisible ? 0.2 : 1)
    map.setPaintProperty('map-world-line', 'line-opacity', chinaVisible ? 0.35 : 0.8)

    if (isWorldMode) {
      const coordinate = selectedCountry ? countryCoordinates[selectedCountry] : null
      map.flyTo({ center: coordinate || worldCenter, zoom: coordinate ? 2.6 : 1.2, duration: 560 })
    } else {
      const coordinate = selectedProvince ? provinceCentersRef.current.get(selectedProvince) : null
      map.flyTo({ center: coordinate || chinaCenter, zoom: coordinate ? 4.2 : 3.2, duration: 560 })
    }
  }, [isWorldMode, mapReady, selectedCountry, selectedProvince])

  // Labels are DOM markers so Chinese names render without a glyph server.
  useEffect(() => {
    const map = mapRef.current
    const maplibregl = maplibreRef.current
    if (!mapReady || !map || !maplibregl || !overview || !geoVersion) return

    markersRef.current.forEach((marker) => marker.remove())
    markersRef.current = []

    const addMarker = (coordinate: [number, number], text: string, className: string, anchor: string, offset: [number, number]) => {
      const element = document.createElement('div')
      element.className = className
      element.textContent = text
      const marker = new maplibregl.Marker({ element, anchor, offset })
        .setLngLat(coordinate)
        .addTo(map)
      markersRef.current.push(marker)
    }

    if (!isWorldMode) {
      const labelLimit = 13
      const ranked = [...overview.regions].sort((first, second) => second.assetCount - first.assetCount)
      const labelled = new Set(ranked.slice(0, labelLimit).map((region) => region.province))
      if (selectedProvince) labelled.add(selectedProvince)
      for (const province of labelled) {
        const feature = provinceFeaturesRef.current.get(province)
        const coordinate = provinceCentersRef.current.get(province)
        if (!feature || !coordinate) continue
        const region = overview.regions.find((item) => item.province === province)
        const isSelected = province === selectedProvince
        addMarker(
          coordinate,
          shortProvinceName(String(feature.properties.name || province)),
          cn('asset-map-label', isSelected && 'asset-map-label-selected', !region?.assetCount && 'asset-map-label-muted'),
          'center',
          [0, 0]
        )
      }

      if (selectedProvince) {
        const region = overview.regions.find((item) => item.province === selectedProvince)
        const cities = [...(region?.cities || [])]
          .filter(
            (city) =>
              !region ||
              municipalityProvinces.has(region.province) ||
              (city.name !== region.province && city.name !== region.provinceLabel)
          )
          .sort((first, second) => second.count - first.count)
          .slice(0, 4)
        for (const city of cities) {
          if (selectedCity && city.name !== selectedCity) continue
          const coordinate = cityFeatureCoordinate(selectedProvince, city.name, provinceCentersRef.current)
          if (!coordinate) continue
          addMarker(
            coordinate,
            city.name,
            cn('asset-map-city-label', city.name === selectedCity && 'asset-map-city-label-selected'),
            'left',
            [8, 2]
          )
        }
      }
    } else {
      const ranked = [...overview.countries]
        .sort((first, second) => (second.assetCount || second.count) - (first.assetCount || first.count))
        .slice(0, 8)
      for (const country of ranked) {
        const key = country.country || country.name
        const coordinate = countryCoordinates[key]
        if (!coordinate) continue
        addMarker(
          coordinate,
          country.countryLabel || country.name,
          cn('asset-map-label', key === selectedCountry && 'asset-map-label-selected'),
          'center',
          [0, 0]
        )
      }
    }

    return () => {
      markersRef.current.forEach((marker) => marker.remove())
      markersRef.current = []
    }
  }, [geoVersion, isWorldMode, mapReady, overview, selectedCity, selectedCountry, selectedProvince])

  /* ---------------------------- interactions ----------------------------- */

  const selectProvince = useCallback((province: string, focus = true) => {
    setScope('located')
    setMode('china')
    setSelectedProvince(province)
    setSelectedCity('')
    setPage(1)
    setDetailTab('assets')
    if (focus) {
      const coordinate = provinceCentersRef.current.get(province)
      if (coordinate) mapRef.current?.flyTo({ center: coordinate, zoom: 4.3, duration: 520 })
    }
  }, [])

  const selectCity = useCallback((province: string, city: string) => {
    setScope('located')
    setMode('china')
    setSelectedProvince(province)
    setSelectedCity(city)
    setPage(1)
    setDetailTab('assets')
    const coordinate = cityFeatureCoordinate(province, city, provinceCentersRef.current)
    if (coordinate) mapRef.current?.flyTo({ center: coordinate, zoom: 5.4, duration: 520 })
  }, [])

  const selectCountry = useCallback((country: string) => {
    setScope('overseas')
    setMode('world')
    setSelectedCountry(country)
    setPage(1)
    setDetailTab('assets')
  }, [])

  useEffect(() => {
    selectProvinceRef.current = selectProvince
    selectCityRef.current = selectCity
    selectCountryRef.current = selectCountry
  }, [selectCity, selectCountry, selectProvince])

  const changeScope = (next: MapScope) => {
    setScope(next)
    setPage(1)
    setDetailTab('assets')
    setSelectedCity('')
    if (next === 'overseas') {
      setMode('world')
      const first = overview?.countries[0]
      setSelectedCountry(first?.country || first?.name || '')
    } else {
      setMode('china')
      if (next === 'located' && !selectedProvince) {
        setSelectedProvince(overview?.regions[0]?.province || '')
      }
      if (next === 'internal' || next === 'unresolved') {
        setSelectedProvince('')
      }
    }
  }

  const refresh = () => {
    setRefreshKey((key) => key + 1)
    setPage(1)
  }

  const openAsset = (asset: AssetMapAsset) => {
    if (asset.assetId) {
      setDetailAssetId(asset.assetId)
      return
    }
    toast.info(copy.legacyAsset)
  }

  const maxLegend = colorMode === 'risk'
    ? Math.max(1, ...(overview?.regions || []).map((region) => region.criticalCount + region.highCount))
    : maxRegionCount

  /* -------------------------------- render ------------------------------- */

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-background">
      <header className="flex flex-none flex-col gap-3 border-b border-border bg-card/70 px-4 py-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-primary/30 bg-primary/10 text-primary">
            <MapPinned className="size-4" />
          </span>
          <div className="min-w-0">
            <div className="flex items-baseline gap-3">
              <h1 className="truncate text-lg font-semibold text-foreground">{copy.title}</h1>
              <span className="text-[11px] text-muted-foreground">{copy.subtitle}</span>
            </div>
            <div className="mt-0.5 flex items-center gap-1.5 text-[11px] text-muted-foreground">
              <span>{copy.updatedAt} {formatTime(overview?.updatedAt, isEnglish)}</span>
              {(overview?.riskAssets?.total || 0) > 0 && (
                <span
                  className="rounded border border-border bg-muted/40 px-1.5 py-0.5 text-[10px]"
                  title={[
                    `${isEnglish ? 'IP' : 'IP'} ${overview?.riskAssets?.ip || 0}`,
                    `${isEnglish ? 'Domain' : '域名'} ${overview?.riskAssets?.domain || 0}`,
                    `${isEnglish ? 'URL' : 'URL'} ${overview?.riskAssets?.url || 0}`,
                    `${isEnglish ? 'Service' : '服务'} ${overview?.riskAssets?.service || 0}`,
                    `${isEnglish ? 'Certificate' : '证书'} ${overview?.riskAssets?.certificate || 0}`,
                    `${copy.critical} ${overview?.riskAssets?.critical || 0}`,
                    `${copy.high} ${overview?.riskAssets?.high || 0}`,
                    `${copy.medium} ${overview?.riskAssets?.medium || 0}`,
                  ].join(' · ')}
                >
                  {isEnglish ? 'Risk assets' : '风险资产'} {formatNumber(overview?.riskAssets?.total || 0, isEnglish)}
                </span>
              )}
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-stretch gap-0.5 rounded-lg border border-border bg-background p-0.5">
            {scopeItems.map((item) => {
              const Icon = item.icon
              const active = scope === item.key
              return (
                <button
                  key={item.key}
                  type="button"
                  onClick={() => changeScope(item.key)}
                  className={cn(
                    'flex min-w-[70px] flex-col items-start gap-0.5 rounded-md border border-transparent px-2.5 py-1 text-left transition-colors',
                    active
                      ? 'border-primary/25 bg-primary/10'
                      : 'hover:bg-muted/70'
                  )}
                  aria-pressed={active}
                >
                  <span className={cn('flex items-center gap-1 text-[10px]', active ? 'text-primary' : 'text-muted-foreground')}>
                    <Icon className="size-2.5" />
                    {item.label}
                  </span>
                  <span className={cn('font-mono text-sm font-semibold leading-none', active ? 'text-foreground' : 'text-foreground/80')}>
                    {formatNumber(item.value, isEnglish)}
                  </span>
                </button>
              )
            })}
          </div>

          <div className="relative hidden xl:block">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              placeholder={copy.searchPlaceholder}
              className="h-8 w-[220px] pl-8 text-xs"
              aria-label={copy.searchPlaceholder}
            />
          </div>

          <Select
            value={windowKey || 'all'}
            onValueChange={(value) => {
              setWindowKey(value === 'all' ? '' : value)
              setPage(1)
            }}
          >
            <SelectTrigger size="sm" className="h-8 w-[116px] text-xs" aria-label={copy.windowAll}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{copy.windowAll}</SelectItem>
              <SelectItem value="7d">{copy.window7d}</SelectItem>
              <SelectItem value="30d">{copy.window30d}</SelectItem>
              <SelectItem value="90d">{copy.window90d}</SelectItem>
            </SelectContent>
          </Select>

          <Button variant="outline" size="sm" className="h-8 gap-1.5 px-2.5" onClick={refresh} disabled={overviewLoading}>
            <RefreshCw className={cn('size-3.5', overviewLoading && 'animate-spin')} />
            <span className="hidden sm:inline">{copy.refresh}</span>
          </Button>
        </div>
      </header>

      {(error || mapError) && (
        <div className="flex flex-none items-center gap-2 border-b border-border bg-amber-500/10 px-4 py-2 text-xs text-amber-700 dark:text-amber-300">
          <AlertCircle className="size-3.5" />
          <span>{error || mapError}</span>
        </div>
      )}

      {!isGeoScope && (
        <div className="flex flex-none items-center gap-2 border-b border-border bg-muted/40 px-4 py-2 text-xs text-muted-foreground">
          <Server className="size-3.5" />
          <span>{scope === 'internal' ? copy.internalOnly : copy.unresolvedOnly}</span>
        </div>
      )}

      <div className="grid min-h-0 flex-1 grid-cols-1 overflow-hidden xl:grid-cols-[240px_minmax(0,1fr)_384px]">
        <aside className="flex min-h-0 flex-col overflow-hidden border-b border-border bg-card/40 xl:border-b-0 xl:border-r">
          <div className="flex flex-none items-baseline justify-between px-4 pb-1 pt-3">
            <span className="text-[11px] font-medium text-muted-foreground">
              {isWorldMode ? copy.countries : copy.provinces}
            </span>
            <span className="text-[10px] text-muted-foreground">
              {formatNumber(isWorldMode ? overview?.overseasIpAssets || 0 : overview?.locatedIpAssets || 0, isEnglish)} ·{' '}
              {rankItems.length} {copy.regionCount}
            </span>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
            {rankItems.map((item, index) => {
              const isCountry = isWorldMode
              const key = isCountry ? (item as AssetMapCountry).country || (item as AssetMapCountry).name : (item as AssetMapRegion).province
              const label = isCountry
                ? (item as AssetMapCountry).countryLabel || (item as AssetMapCountry).name
                : (item as AssetMapRegion).provinceLabel
              const count = isCountry
                ? (item as AssetMapCountry).assetCount || (item as AssetMapCountry).count
                : (item as AssetMapRegion).assetCount
              const active = isCountry ? selectedCountry === key : selectedProvince === key
              const percent = Math.max(2, Math.round((count / rankMax) * 100))
              return (
                <button
                  key={`${isCountry ? 'country' : 'province'}-${key}`}
                  type="button"
                  disabled={rankingDisabled}
                  onClick={() => (isCountry ? selectCountry(key) : selectProvince(key))}
                  className={cn(
                    'relative mb-0.5 flex h-8 w-full items-center gap-2 rounded-md px-2 text-left transition-colors',
                    active ? 'bg-primary/10 ring-1 ring-inset ring-primary/25' : 'hover:bg-muted/60',
                    rankingDisabled && 'cursor-not-allowed opacity-45 hover:bg-transparent'
                  )}
                  aria-pressed={active}
                >
                  <span className="w-4 shrink-0 text-[10px] tabular-nums text-muted-foreground/70">
                    {String(index + 1).padStart(2, '0')}
                  </span>
                  <span className={cn('truncate text-xs', active ? 'font-medium text-foreground' : 'text-foreground/85')}>
                    {label}
                  </span>
                  <span className={cn('ml-auto shrink-0 font-mono text-[11px]', active ? 'text-primary' : 'text-muted-foreground')}>
                    {formatNumber(count, isEnglish)}
                  </span>
                  <span className="absolute inset-x-2 bottom-1 h-px overflow-hidden rounded-full bg-primary/10">
                    <span
                      className={cn('block h-full rounded-full', active ? 'bg-primary' : 'bg-primary/30')}
                      style={{ width: `${percent}%` }}
                    />
                  </span>
                </button>
              )
            })}
            {!overviewLoading && !rankItems.length && (
              <div className="px-3 py-8 text-center text-[11px] text-muted-foreground">
                {isWorldMode ? copy.noData : copy.noData}
              </div>
            )}
          </div>

        </aside>

        <main className="asset-map-canvas-shell relative min-h-[520px] overflow-hidden xl:min-h-0">
          <div className="asset-map-grid pointer-events-none absolute inset-0" />
          <div className="absolute inset-0 z-0">
            <div
              ref={mapContainerRef}
              className="h-full w-full"
              aria-label={copy.title}
            />
          </div>

          <div className="pointer-events-none absolute inset-x-0 top-0 z-10 flex items-start justify-between p-3">
            <div className="pointer-events-auto flex items-center gap-1 rounded-md border border-border bg-background/90 p-0.5 shadow-sm backdrop-blur">
              <span className="px-2 text-[10px] text-muted-foreground">{copy.colorBy}</span>
              <button
                type="button"
                onClick={() => setColorMode('assets')}
                className={cn('h-6 rounded px-2 text-[10px]', colorMode === 'assets' ? 'bg-primary/15 text-primary' : 'text-muted-foreground')}
              >
                {copy.colorByAssets}
              </button>
              <button
                type="button"
                onClick={() => setColorMode('risk')}
                disabled={!hasRiskData}
                title={!hasRiskData ? copy.noRiskData : copy.legendRisk}
                className={cn(
                  'h-6 rounded px-2 text-[10px]',
                  colorMode === 'risk' ? 'bg-primary/15 text-primary' : 'text-muted-foreground',
                  !hasRiskData && 'cursor-not-allowed opacity-35'
                )}
              >
                {copy.colorByRisk}
              </button>
            </div>
          </div>

          <div className="pointer-events-none absolute bottom-3 left-3 z-10 w-[196px] rounded-lg border border-border bg-background/90 p-2.5 shadow-sm backdrop-blur">
            <div className="flex items-center justify-between text-[10px] text-muted-foreground">
              <span>{colorMode === 'risk' ? copy.legendRisk : copy.legendTitle}</span>
              <span className="font-mono text-foreground/80">{formatNumber(maxLegend, isEnglish)}</span>
            </div>
            <div
              className="mt-1.5 h-1.5 rounded-full"
              style={{ background: `linear-gradient(90deg, ${palette.scale.join(',')})` }}
            />
            <div className="mt-1 flex justify-between font-mono text-[9px] text-muted-foreground">
              <span>0</span>
              <span>{formatNumber(Math.round(maxLegend * 0.5), isEnglish)}</span>
              <span>{formatNumber(maxLegend, isEnglish)}</span>
            </div>
            <div className="mt-2 flex items-center gap-1.5 text-[9px] text-muted-foreground">
              <span className="size-1.5 rounded-full bg-primary" />
              <span className="size-2 rounded-full bg-primary/70" />
              <span className="size-2.5 rounded-full bg-primary/50" />
              {isEnglish ? 'City points by volume' : '城市点 · 大小对应资产数'}
            </div>
          </div>

          {showEmptyMap && (
            <div className="absolute inset-0 z-20 flex items-center justify-center p-6">
              <div className="max-w-sm rounded-xl border border-border bg-background/95 p-5 text-center shadow-lg">
                <MapPinned className="mx-auto size-6 text-muted-foreground" />
                <div className="mt-2 text-sm font-medium text-foreground">{copy.noData}</div>
                <p className="mt-1 text-xs text-muted-foreground">{copy.noDataHint}</p>
                <div className="mt-3 flex items-center justify-center gap-2 text-[11px] text-muted-foreground">
                  <span>{copy.internal} {formatNumber(overview?.nonPublicIpAssets || 0, isEnglish)}</span>
                  <span>·</span>
                  <span>{copy.unresolved} {formatNumber(overview?.unresolvedIpAssets || 0, isEnglish)}</span>
                </div>
                <Button asChild size="sm" variant="outline" className="mt-3 h-7 text-xs">
                  <Link href="/asset-center">{isEnglish ? 'Open asset ledger' : '前往资产台账'}</Link>
                </Button>
              </div>
            </div>
          )}

          {overviewLoading && !mapError && (
            <div className="absolute inset-0 z-20 flex items-center justify-center bg-background/40 backdrop-blur-[1px]">
              <Spinner className="size-6" />
            </div>
          )}
          {!mapReady && !overviewLoading && !mapError && (
            <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center">
              <Spinner className="size-5 opacity-60" />
            </div>
          )}
        </main>

        <aside className="flex min-h-0 flex-col overflow-hidden border-t border-border bg-card/40 xl:border-l xl:border-t-0">
          <div className="flex-none border-b border-border px-4 pb-3 pt-3">
            <div className="truncate text-[10px] uppercase tracking-wide text-muted-foreground">
              {rankingDisabled
                ? `${copy.internal} / ${copy.unresolved}`
                : isWorldMode
                ? `${copy.overseas} / ${detailTitle}`
                : `${copy.domestic} / ${detailTitle}`}
            </div>
            <div className="mt-0.5 flex items-baseline gap-2">
              <h2 className="truncate text-base font-semibold text-foreground">{detailTitle}</h2>
              <span className="shrink-0 font-mono text-[11px] text-primary">
                {formatNumber(detailAssetCount, isEnglish)} {copy.assets}
              </span>
              {(detailCritical > 0 || detailHigh > 0 || detailMedium > 0) && (
                <span
                  className="ml-auto shrink-0 rounded border border-amber-500/25 bg-amber-500/10 px-1.5 py-0.5 text-[10px] text-amber-700 dark:text-amber-300"
                  title={
                    detailInherited > 0
                      ? `${copy.directRisk} ${(isWorldMode ? detailCountry?.directCritical || 0 : detailRegion?.directCritical || 0)} ${copy.critical} / ${(isWorldMode ? detailCountry?.directHigh || 0 : detailRegion?.directHigh || 0)} ${copy.high} / ${(isWorldMode ? detailCountry?.directMedium || 0 : detailRegion?.directMedium || 0)} ${copy.medium} · ${copy.inheritedRisk} ${detailInherited}`
                      : undefined
                  }
                >
                  {copy.critical} {detailCritical} · {copy.high} {detailHigh}
                  {detailMedium > 0 ? ` · ${copy.medium} ${detailMedium}` : ''}
                  {detailInherited > 0 ? ` · ${copy.inherited} ${detailInherited}` : ''}
                </span>
              )}
            </div>
            <div className="mt-1 flex min-w-0 items-center gap-1 text-[10px] text-muted-foreground">
              <span className="truncate">{detailSubtitle || copy.noData}</span>
              {selectedCity && scope === 'located' && (
                <button
                  type="button"
                  onClick={() => {
                    setSelectedCity('')
                    setPage(1)
                  }}
                  className="ml-1 inline-flex shrink-0 items-center gap-0.5 rounded border border-border px-1 py-0.5 text-[9px] hover:bg-muted"
                  aria-label={copy.clearCity}
                >
                  <X className="size-2.5" />
                  {copy.clearCity}
                </button>
              )}
            </div>
          </div>

          <div className="flex h-9 flex-none items-center gap-4 border-b border-border px-4">
            {([
              { key: 'assets' as const, label: copy.tabAssets, count: detailAssetCount },
              { key: 'cities' as const, label: copy.tabCities, count: detailCities.length },
              { key: 'operators' as const, label: copy.tabOperators, count: detailOperators.length },
            ]).map((tab) => (
              <button
                key={tab.key}
                type="button"
                onClick={() => setDetailTab(tab.key)}
                className={cn(
                  'flex h-9 items-center gap-1 border-b-2 text-[11px] transition-colors',
                  detailTab === tab.key
                    ? 'border-primary text-foreground'
                    : 'border-transparent text-muted-foreground hover:text-foreground'
                )}
              >
                {tab.label}
                {tab.key === 'assets' && (
                  <span className="rounded bg-muted px-1 text-[9px] text-muted-foreground">{formatNumber(tab.count, isEnglish)}</span>
                )}
              </button>
            ))}
          </div>

          {detailTab === 'assets' && (
            <>
              <div className="flex flex-none items-center gap-1 border-b border-border px-4 py-2">
                {([
                  { key: 'all' as const, label: copy.all },
                  { key: 'critical' as const, label: copy.critical },
                  { key: 'high' as const, label: copy.high },
                  { key: 'medium' as const, label: copy.medium },
                  { key: 'any' as const, label: copy.risky },
                ]).map((option) => (
                  <button
                    key={option.key}
                    type="button"
                    onClick={() => {
                      setRisk(option.key)
                      setPage(1)
                    }}
                    className={cn(
                      'rounded border px-1.5 py-0.5 text-[10px] transition-colors',
                      risk === option.key
                        ? 'border-primary/30 bg-primary/10 text-primary'
                        : 'border-border bg-background text-muted-foreground hover:bg-muted'
                    )}
                  >
                    {option.label}
                  </button>
                ))}
                <span className="ml-auto flex items-center gap-1 text-[10px] text-muted-foreground">
                  <ShieldAlert className="size-3" />
                  {copy.sortRisk}
                </span>
              </div>

              <div className="min-h-0 flex-1 overflow-y-auto p-2">
                {assets.map((asset) => {
                  const highestRisk: 'critical' | 'high' | 'medium' | null =
                    asset.criticalCount > 0 ? 'critical' : asset.highCount > 0 ? 'high' : asset.mediumCount > 0 ? 'medium' : null
                  return (
                    <button
                      key={`${asset.value}-${asset.updatedAt}`}
                      type="button"
                      onClick={() => openAsset(asset)}
                      className="group flex w-full items-start gap-2 rounded-md border border-transparent px-2 py-2 text-left transition-colors hover:border-border hover:bg-muted/40"
                      title={`${copy.directRisk} ${asset.directCritical || 0} ${copy.critical} / ${asset.directHigh || 0} ${copy.high} / ${asset.directMedium || 0} ${copy.medium} · ${copy.inheritedRisk} ${asset.inheritedCritical || 0} ${copy.critical} / ${asset.inheritedHigh || 0} ${copy.high} / ${asset.inheritedMedium || 0} ${copy.medium}`}
                    >
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <span className="truncate font-mono text-xs text-foreground">{asset.value}</span>
                          {highestRisk === 'critical' && (
                            <span className="shrink-0 rounded border border-red-500/30 bg-red-500/10 px-1 py-0.5 text-[9px] leading-none text-red-600 dark:text-red-300">
                              {copy.critical}
                            </span>
                          )}
                          {highestRisk === 'high' && (
                            <span className="shrink-0 rounded border border-amber-500/30 bg-amber-500/10 px-1 py-0.5 text-[9px] leading-none text-amber-700 dark:text-amber-300">
                              {copy.high}
                            </span>
                          )}
                          {highestRisk === 'medium' && (
                            <span className="shrink-0 rounded border border-yellow-500/30 bg-yellow-500/10 px-1 py-0.5 text-[9px] leading-none text-yellow-700 dark:text-yellow-300">
                              {copy.medium}
                            </span>
                          )}
                          {(asset.inheritedCritical || 0) + (asset.inheritedHigh || 0) + (asset.inheritedMedium || 0) > 0 && (
                            <span className="shrink-0 rounded border border-border bg-muted/50 px-1 py-0.5 text-[9px] leading-none text-muted-foreground">
                              {(asset.inheritedCritical || 0) + (asset.inheritedHigh || 0) + (asset.inheritedMedium || 0)} {copy.inherited}
                            </span>
                          )}
                          <span className="ml-auto shrink-0 font-mono text-[9px] text-muted-foreground">
                            {asset.ports?.slice(0, 2).join(' · ') || asset.services?.[0] || ''}
                          </span>
                        </div>
                        <div className="mt-1 flex min-w-0 items-center gap-1 text-[10px] text-muted-foreground">
                          <span className="truncate">
                            {asset.city || (asset.country && asset.country !== '中国' ? asset.country : '') || copy.unknownCity}
                          </span>
                          <span className="text-border">·</span>
                          <span className="truncate">{asset.operator || '—'}</span>
                          <span className="text-border">·</span>
                          <span className="truncate font-mono">{asset.asn || '—'}</span>
                          <span className="text-border">·</span>
                          <span className="shrink-0">{formatTime(asset.lastSeenAt || asset.updatedAt, isEnglish)}</span>
                        </div>
                      </div>
                      <ChevronRight className="mt-1 size-3.5 shrink-0 text-muted-foreground/50 group-hover:text-muted-foreground" />
                    </button>
                  )
                })}

                {assetsLoading && (
                  <div className="flex items-center justify-center py-6">
                    <Spinner className="size-4" />
                  </div>
                )}

                {!assetsLoading && !assets.length && (
                  <div className="px-3 py-10 text-center text-[11px] text-muted-foreground">
                    {copy.noAssets}
                  </div>
                )}
              </div>

              <div className="flex flex-none items-center gap-2 border-t border-border px-4 py-2">
                <span className="text-[10px] text-muted-foreground">
                  {copy.shown} {formatNumber(assets.length, isEnglish)} / {formatNumber(assetsTotal, isEnglish)}
                </span>
                {assets.length < assetsTotal && (
                  <button
                    type="button"
                    onClick={() => setPage((current) => current + 1)}
                    disabled={assetsLoading}
                    className="ml-auto rounded border border-border px-1.5 py-0.5 text-[10px] text-muted-foreground hover:bg-muted disabled:opacity-50"
                  >
                    {copy.loadMore}
                  </button>
                )}
                <Link
                  href="/asset-center"
                  className={cn(
                    'flex items-center gap-0.5 text-[10px] text-primary hover:underline',
                    assets.length < assetsTotal ? '' : 'ml-auto'
                  )}
                >
                  {copy.ledger}
                  <ArrowUpRight className="size-2.5" />
                </Link>
              </div>
            </>
          )}

          {detailTab === 'cities' && (
            <div className="min-h-0 flex-1 overflow-y-auto p-4">
              <div className="mb-2 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <Layers3 className="size-3.5 text-primary" />
                {copy.cityDistribution}
              </div>
              <MiniBarList items={detailCities} emptyText={copy.noStats} isEnglish={isEnglish} />
            </div>
          )}

          {detailTab === 'operators' && (
            <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
              <div>
                <div className="mb-2 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                  <Wifi className="size-3.5 text-primary" />
                  {copy.operatorDistribution}
                </div>
                <MiniBarList items={detailOperators} emptyText={copy.noStats} isEnglish={isEnglish} />
              </div>
              <div>
                <div className="mb-2 flex items-center gap-1.5 text-[11px] text-muted-foreground">
                  <Server className="size-3.5 text-primary" />
                  {copy.asnDistribution}
                </div>
                <MiniBarList items={detailAsns} emptyText={copy.noStats} isEnglish={isEnglish} />
              </div>
            </div>
          )}
        </aside>
      </div>

      <AssetDetailSheet
        assetId={detailAssetId}
        open={Boolean(detailAssetId)}
        onOpenChange={(open) => {
          if (!open) setDetailAssetId(null)
        }}
        onChanged={() => setRefreshKey((key) => key + 1)}
      />
    </div>
  )
}

/* -------------------------------------------------------------------------- */
/* Small presentational helpers                                               */
/* -------------------------------------------------------------------------- */

function MiniBarList({
  items,
  emptyText,
  isEnglish,
}: {
  items: DimensionStat[]
  emptyText: string
  isEnglish: boolean
}) {
  if (!items.length) {
    return <div className="py-6 text-center text-[11px] text-muted-foreground">{emptyText}</div>
  }
  const max = Math.max(1, ...items.map((item) => item.count))
  return (
    <div className="space-y-1.5">
      {items.map((item) => (
        <div key={item.name} className="relative overflow-hidden rounded px-1.5 py-1">
          <span
            className="absolute inset-y-0 left-0 bg-primary/10"
            style={{ width: `${Math.max(3, (item.count / max) * 100)}%` }}
          />
          <div className="relative flex items-center justify-between gap-2 text-[11px]">
            <span className="truncate text-foreground/85">{item.name}</span>
            <span className="shrink-0 font-mono text-muted-foreground">{formatNumber(item.count, isEnglish)}</span>
          </div>
        </div>
      ))}
    </div>
  )
}
