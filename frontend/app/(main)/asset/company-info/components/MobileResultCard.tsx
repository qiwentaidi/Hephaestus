import type { ReactNode } from 'react'

export function MobileResultCard({ title, fields, children }: {
  title: ReactNode
  fields: { label: string; value: ReactNode }[]
  children?: ReactNode
}) {
  return (
    <article className="min-w-0 rounded-lg border bg-card p-3">
      <div className="mb-3 min-w-0 break-words font-medium [overflow-wrap:anywhere]">{title}</div>
      <dl className="space-y-2 text-sm">
        {fields.map(({ label, value }) => (
          <div key={label} className="grid min-w-0 grid-cols-[5rem_minmax(0,1fr)] gap-3">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="min-w-0 whitespace-pre-wrap [overflow-wrap:anywhere]">{value === '' || value == null ? '—' : value}</dd>
          </div>
        ))}
      </dl>
      {children}
    </article>
  )
}

export function MobileResultList({ empty, emptyText, children }: {
  empty: boolean
  emptyText: string
  children: ReactNode
}) {
  return (
    <div className="min-w-0 space-y-3 md:hidden">
      {empty ? <p className="py-8 text-center text-sm text-muted-foreground">{emptyText}</p> : children}
    </div>
  )
}
