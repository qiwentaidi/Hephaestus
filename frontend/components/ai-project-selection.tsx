'use client'

import Link from 'next/link'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { AIWorkbenchProject } from '@/lib/api/ai-workbench'
import { useI18n } from '@/lib/i18n'

export function matchingAIProjects(projects: AIWorkbenchProject[], name: string) {
  const normalized = name.trim()
  return normalized ? projects.filter(project => project.name.trim() === normalized) : []
}

export function AIProjectSelection({ id, projects, projectId, onChange, orchestration, name, disabled }: {
  id: string
  projects: AIWorkbenchProject[]
  projectId: string
  onChange: (value: string) => void
  orchestration: boolean
  name: string
  disabled?: boolean
}) {
  const { t } = useI18n()
  if (orchestration) {
    const matches = matchingAIProjects(projects, name)
    return <div className="space-y-2 text-sm text-muted-foreground">
      <p>{t.aiPlan.newProjectOnly}</p>
      {matches.length > 0 && <div role="alert" className="space-y-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-3">
        <p>{t.aiPlan.existingProjectHint}</p>
        {matches.map(project => <Link key={project.id} className="block underline underline-offset-4" href={`/ai/workbench/project?project=${encodeURIComponent(project.id)}&tab=plan`}>
          {t.aiPlan.openProject} · {project.name} · {project.id.slice(0, 8)}
        </Link>)}
      </div>}
    </div>
  }
  return <div className="space-y-2">
    <Label htmlFor={id}>{t.aiBatch.project}</Label>
    <Select value={projectId || '__new__'} onValueChange={value => onChange(value === '__new__' ? '' : value)} disabled={disabled}>
      <SelectTrigger id={id} className="w-full"><SelectValue /></SelectTrigger>
      <SelectContent>
        <SelectItem value="__new__">{t.aiBatch.autoProject}</SelectItem>
        {projects.map(project => <SelectItem key={project.id} value={project.id}>{project.name} · {project.id.slice(0, 8)}</SelectItem>)}
      </SelectContent>
    </Select>
  </div>
}
