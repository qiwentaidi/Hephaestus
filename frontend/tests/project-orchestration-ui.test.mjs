import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

test('project page owns the orchestration tab', () => {
  const project = read('app/(main)/ai/workbench/project/ProjectClient.tsx')
  assert.match(project, /plan: \['编排', 'Plan'\]/)
  assert.match(project, /<PlanWorkspace projectId=\{id\}/)
})

test('orchestration sessions are grouped with main agent and planner pinned above workers', () => {
  const workbench = read('app/(main)/ai/workbench/page.tsx')
  assert.match(workbench, /agentRole === 'worker'/)
  assert.match(workbench, /data-agent-group=\{key\}/)
  assert.match(workbench, /section\('main'/)
  assert.match(workbench, /section\('planner'/)
  assert.match(workbench, /section\('workers'/)
  assert.match(workbench, /data-pinned="true"/)
  assert.doesNotMatch(workbench, /workersByParent/)
  assert.match(workbench, /workerReadOnlyHint/)
})

test('controller is the writable orchestration entry', () => {
  const workbench = read('app/(main)/ai/workbench/page.tsx')
  const project = read('app/(main)/ai/workbench/project/components/PlanWorkspace.tsx')
  const detail = read('app/(main)/ai/tasks/components/PlanTaskDetail.tsx')
  assert.match(workbench, /plannerReadOnlyHint/)
  assert.match(workbench, /openPlanController/)
  assert.match(workbench, /ensureAIPlanRunController/)
  assert.match(project, /ensureAIPlanRunController/)
  assert.match(project, /openController/)
  assert.match(detail, /ensureAIPlanRunController/)
  assert.match(detail, /openController/)
})

test('standalone plan page is no longer linked', () => {
  const layout = read('app/(main)/MainLayoutClient.tsx')
  assert.doesNotMatch(layout, /ai\/plans/)
})

test('task management unifies batch and orchestration tasks', () => {
  const tasks = read('app/(main)/ai/tasks/components/TaskManagement.tsx')
  const create = read('app/(main)/ai/tasks/components/TaskCreateDialog.tsx')
  const detail = read('app/(main)/ai/tasks/components/PlanTaskDetail.tsx')
  assert.match(tasks, /unifiedTasks/)
  assert.match(tasks, /taskStatusGroup/)
  assert.match(tasks, /listAIPlanRunSummaries/)
  assert.match(tasks, /<TaskCreateDialog/)
  assert.match(tasks, /<PlanTaskDetail/)
  assert.doesNotMatch(tasks, /asset-center/)
  assert.match(create, /taskMode === 'plan'/)
  assert.match(create, /createAIPlanRun/)
  assert.match(detail, /PlanStatusBadge/)
})

test('evidence index pages every fact and finding from the server', () => {
  const evidence = read('app/(main)/ai/workbench/project/components/EvidenceWorkspace.tsx')
  const page = read('app/(main)/ai/workbench/project/ProjectClient.tsx')
  const api = read('lib/api/ai-workbench.ts')
  assert.match(evidence, /listAIWorkbenchProjectEvidence/)
  assert.match(evidence, /getAIWorkbenchProjectEvidenceEntry/)
  assert.match(evidence, /<PaginationPro/)
  assert.match(evidence, /source: source === 'all' \? '' : source/)
  assert.match(page, /<EvidenceWorkspace key=\{source\} projectId=\{id\}/)
  assert.match(page, /evidenceCounts/)
  assert.match(api, /AIWorkbenchProjectEvidencePage/)
  assert.match(api, /projects\/\$\{encodeURIComponent\(projectId\)\}\/evidence/)
})

test('orchestration creation links read-only source tasks', () => {
  const dialog = read('app/(main)/ai/tasks/components/TaskCreateDialog.tsx')
  const api = read('lib/api/ai-plan.ts')
  assert.match(dialog, /listAIPlanRunSummaries\(\)/)
  assert.match(dialog, /p\.sourceTasksHint/)
  assert.match(dialog, /sourceTaskIds/)
  assert.match(dialog, /isOrchestrationRunId\(item\.run\.id\) && !item\.run\.readOnly/)
  assert.match(api, /sourceTaskIds\?: string\[\]/)
})

test('plan run surfaces inherited source tasks on card and detail', () => {
  const detail = read('app/(main)/ai/tasks/components/PlanTaskDetail.tsx')
  const workspace = read('app/(main)/ai/workbench/project/components/PlanWorkspace.tsx')
  assert.match(detail, /item\.sourceTaskIds/)
  assert.match(workspace, /sourceTaskIds = \[\]/)
})
