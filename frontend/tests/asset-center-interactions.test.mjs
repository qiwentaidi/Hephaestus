import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../lib/asset-center-filters.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const { buildAssetTopologyQuery } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)

test('asset status filter is included in topology requests and preserves pagination', () => {
  const query = buildAssetTopologyQuery({ keyword: 'example.com', type: 'domain', status: 'stale', port: ' 443 ', service: '', technology: 'nginx' }, 2, 25)
  assert.equal(query.get('status'), 'stale')
  assert.equal(query.get('port'), '443')
  assert.equal(query.get('page'), '2')
  assert.equal(query.get('pageSize'), '25')
})

test('asset center UI keeps stable interaction hooks for browser acceptance', () => {
  const inventory = readFileSync(new URL('../app/(main)/asset-center/components/AssetInventoryView.tsx', import.meta.url), 'utf8')
  const detail = readFileSync(new URL('../app/(main)/asset-center/components/AssetDetailSheet.tsx', import.meta.url), 'utf8')
  assert.match(inventory, /data-testid="asset-status-filter"/)
  assert.match(inventory, /setSelectedAssetId\(row\.assetId\)/)
  assert.match(detail, /data-testid="asset-detail-sheet"/)
  assert.match(detail, /listUnifiedAssetRelations\(assetId/)
  assert.match(detail, /getUnifiedAssetDetail\(assetId/)
  assert.doesNotMatch(detail, /listUnifiedAssetChanges/)
})


test('asset center defaults to categorized entities and wires the scan entry', () => {
  const page = readFileSync(new URL('../app/(main)/asset-center/page.tsx', import.meta.url), 'utf8')
  const ledger = readFileSync(new URL('../app/(main)/asset-center/components/AssetLedgerView.tsx', import.meta.url), 'utf8')
  const scanDialog = readFileSync(new URL('../app/(main)/asset-center/components/AssetScanDialog.tsx', import.meta.url), 'utf8')
  assert.match(page, /AssetLedgerView/)
  assert.match(ledger, /listAssetLedgerEntities/)
  assert.match(ledger, /TabsTrigger/)
  assert.match(ledger, /tabWebsite/)
  assert.match(ledger, /importAssetLedger/)
  assert.match(ledger, /createAssetLedger/)
  assert.match(ledger, /archiveUnavailable/)
  assert.match(ledger, /TooltipTrigger/)
  assert.match(ledger, /selectedEntities/)
  assert.match(ledger, /selectionHeader/)
  assert.match(ledger, /AssetScanDialog/)
  assert.match(ledger, /setScanOpen\(true\)/)
  assert.doesNotMatch(ledger, /scanPending/)
  assert.match(scanDialog, /buildCombinationPresetCards/)
  assert.match(scanDialog, /buildTemplateCombinationDraft/)
  assert.match(scanDialog, /createAIBatchQueue/)
  assert.match(scanDialog, /deriveCombinationTargets/)
  assert.match(scanDialog, /createAIPlanRun/)
  assert.match(scanDialog, /aiExecution/)
  assert.match(scanDialog, /asset-scan-plan-objective/)
  assert.match(scanDialog, /<Textarea/)
  assert.doesNotMatch(scanDialog, />\s*PRO\s*</)
  assert.match(scanDialog, /asset-scan-task-name/)
  assert.doesNotMatch(scanDialog, /asset-scan-combination-name|asset-scan-ai-name/)
  assert.match(scanDialog, /ai\/workbench\/project\?project=.*tab=plan/)
  assert.match(scanDialog, /SheetContent/)
  assert.match(scanDialog, /savedTemplateCards/)
  assert.match(scanDialog, /TemplateCard/)
})
