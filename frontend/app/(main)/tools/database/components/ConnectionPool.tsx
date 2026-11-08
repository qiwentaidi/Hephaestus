'use client'

import { useI18n } from '@/lib/i18n'
import { DatabaseConnection } from '../types'
import { ConnectionCard } from './ConnectionCard'

interface ConnectionPoolProps {
  connections: DatabaseConnection[]
  onConnectionUpdate: (nanoid: string, updates: Partial<DatabaseConnection>) => void
  onEdit: (connection: DatabaseConnection) => void
  onConnect: (connection: DatabaseConnection) => void
  onDelete: (nanoid: string) => void
}

export function ConnectionPool({
  connections,
  onConnectionUpdate,
  onEdit,
  onConnect,
  onDelete,
}: ConnectionPoolProps) {
  const { t } = useI18n()
  if (connections.length === 0) {
    return (
      <div className="flex items-center justify-center h-[calc(100vh-300px)] text-muted-foreground">
        <p>{t.tools.database.pool.empty}</p>
      </div>
    )
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      {connections.map((conn) => (
        <ConnectionCard
          key={conn.nanoid}
          connection={conn}
          onUpdate={(updates) => onConnectionUpdate(conn.nanoid, updates)}
          onEdit={() => onEdit(conn)}
          onConnect={() => onConnect(conn)}
          onDelete={() => onDelete(conn.nanoid)}
        />
      ))}
    </div>
  )
}
