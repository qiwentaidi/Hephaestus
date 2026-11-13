'use client'
import { DetailResultToolbar } from './DetailResultToolbar'

import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  ToggleGroup,
  ToggleGroupItem,
} from '@/components/ui/toggle-group'
import { Search, LayoutGrid, LayoutList } from 'lucide-react'
import { useI18n } from '@/lib/i18n'
import { FingerprintIconAggregation } from './FingerprintIconAggregation'
import { MultiSelectFilter } from '@/components/ui/multi-select-filter'

interface FingerprintToolbarProps {
  searchQuery: string
  onSearchChange: (value: string) => void
  statusFilter: string[]
  onStatusFilterChange: (value: string[]) => void
  highRiskFilter: string
  onHighRiskFilterChange: (value: string) => void
  taskId?: string
  version?: number
  currentLayout?: 'table' | 'card'
  onLayoutChange?: (layout: 'table' | 'card') => void
}

export function FingerprintToolbar({
  searchQuery,
  onSearchChange,
  statusFilter,
  onStatusFilterChange,
  highRiskFilter,
  onHighRiskFilterChange,
  taskId,
  version,
  currentLayout,
  onLayoutChange,
}: FingerprintToolbarProps) {
  const { t } = useI18n()

  return (
    <DetailResultToolbar activeCount={Number(Boolean(searchQuery.trim())) + Number(statusFilter.length > 0) + Number(highRiskFilter !== 'all')} onReset={() => { onSearchChange(''); onStatusFilterChange([]); onHighRiskFilterChange('all') }}>
<div className="flex flex-wrap items-center gap-2 md:flex-nowrap md:gap-4">
      <div className="hidden md:block">
        <FingerprintIconAggregation taskId={taskId} version={version} onSelectHash={onSearchChange} />
      </div>
      <div className="relative w-full min-w-0 flex-none md:flex-1">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder={t.vulnScan.fingerprint.searchPlaceholder}
          value={searchQuery}
          onChange={(e) => onSearchChange(e.target.value)}
          className="pl-9"
        />
      </div>

      <MultiSelectFilter
        values={statusFilter}
        options={[
          { value: '2xx', label: '2xx' },
          { value: '3xx', label: '3xx' },
          { value: '4xx', label: '4xx' },
          { value: '5xx', label: '5xx' },
        ]}
        allLabel={t.vulnScan.fingerprint.allStatus}
        countLabel={t.common.selectedCount}
        removeLabel={t.common.remove}
        clearLabel={t.common.clear}
        ariaLabel={t.vulnScan.fingerprint.statusFilter}
        onChange={onStatusFilterChange}
        className="min-h-11 w-[calc(50%-0.25rem)] md:min-h-0 md:w-[180px]"
      />

      <Select value={highRiskFilter} onValueChange={onHighRiskFilterChange}>
        <SelectTrigger className="min-h-11 w-[calc(50%-0.25rem)] md:min-h-0 md:w-[180px]">
          <SelectValue placeholder="高风险指纹" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">全部指纹</SelectItem>
          <SelectItem value="with_high_risk">存在高风险指纹</SelectItem>
          <SelectItem value="without_high_risk">无高风险指纹</SelectItem>
        </SelectContent>
      </Select>

      {onLayoutChange && (
        <ToggleGroup
          type="single"
          value={currentLayout}
          onValueChange={(value) => {
            if (value === 'card' || value === 'table') {
              onLayoutChange(value)
            }
          }}
          className="hidden h-9 items-center justify-center rounded-lg border border-border bg-muted/70 p-1 text-muted-foreground md:inline-flex"
        >
          <ToggleGroupItem
            value="table"
            aria-label={t.vulnScan.fingerprint.tableLayout}
            className="h-11 w-11 md:h-7 md:w-7 rounded-md data-[state=on]:bg-primary data-[state=on]:text-primary-foreground hover:bg-muted"
          >
            <LayoutList className="h-4 w-4" />
          </ToggleGroupItem>
          <ToggleGroupItem
            value="card"
            aria-label={t.vulnScan.fingerprint.cardLayout}
            className="h-11 w-11 md:h-7 md:w-7 rounded-md data-[state=on]:bg-primary data-[state=on]:text-primary-foreground hover:bg-muted"
          >
            <LayoutGrid className="h-4 w-4" />
          </ToggleGroupItem>
        </ToggleGroup>
      )}
    </div>
</DetailResultToolbar>
  )
}
