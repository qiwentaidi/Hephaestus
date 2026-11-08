'use client'

import { useState } from 'react'
import { Save } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import toast from '@/lib/toast'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import type { DatabaseConnection } from '../types'
import { DatabaseIconComponent, getDbConfig } from '../utils/dbIcons'

interface ConnectionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  connection: DatabaseConnection | null
  onSaved: () => void
}

const defaultPorts: { [key: string]: number } = {
  mysql: 3306,
  postgres: 5432,
  mssql: 1433,
  oracle: 1521,
  mongodb: 27017,
  redis: 6379,
}

const defaultConnectionForm: Partial<DatabaseConnection> = {
  scheme: 'mysql',
  host: '',
  port: 3306,
  username: '',
  password: '',
  serverName: '',
  notes: '',
}

const getInitialFormData = (connection: DatabaseConnection | null): Partial<DatabaseConnection> => (
  connection ? { ...connection } : { ...defaultConnectionForm }
)

export function ConnectionDialog({ open, onOpenChange, connection, onSaved }: ConnectionDialogProps) {
  const { t } = useI18n()
  const [loading, setLoading] = useState(false)
  const [formState, setFormState] = useState<{ key: string; data: Partial<DatabaseConnection> }>({
    key: '',
    data: { ...defaultConnectionForm },
  })
  const formKey = open ? (connection ? `edit:${connection.nanoid}` : 'new') : 'closed'
  const formData = formState.key === formKey ? formState.data : getInitialFormData(connection)

  const setFormData = (next: Partial<DatabaseConnection>) => {
    setFormState({ key: formKey, data: next })
  }

  const handleSchemeChange = (value: string) => {
    setFormData({
      ...formData,
      scheme: value,
      port: defaultPorts[value] || 3306,
    })
  }

  const handleSave = async () => {
    const requiresUsername = !['mongodb', 'redis'].includes((formData.scheme || '').toLowerCase())
    if (!formData.host || !formData.port || (requiresUsername && !formData.username)) {
      toast.error({
        title: t.tools.database.connectionDialog.validationFailedTitle,
        message: requiresUsername
          ? t.tools.database.connectionDialog.requiredHostPortUser
          : t.tools.database.connectionDialog.requiredHostPort,
      })
      return
    }

    setLoading(true)
    try {
      const method = connection ? 'PUT' : 'POST'
      const response = await fetchWithAuth(`/api/v1/database/connection`, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(formData),
      })

      const data = await response.json()
      if (data.success) {
        toast.success({
          title: t.tools.database.toasts.saveSuccessTitle,
          message: connection
            ? t.tools.database.connectionDialog.updatedMessage
            : t.tools.database.connectionDialog.addedMessage,
        })
        onSaved()
      } else {
        toast.error({
          title: t.tools.database.connectionDialog.saveFailedTitle,
          message: data.error || t.tools.database.connectionDialog.saveFailedMessage,
        })
      }
    } catch (error) {
      toast.error({
        title: t.tools.database.connectionDialog.saveFailedTitle,
        message:
          error instanceof Error
            ? error.message
            : t.tools.database.toasts.networkError,
      })
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          setFormState({ key: '', data: { ...defaultConnectionForm } })
        }
        onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {connection
              ? t.tools.database.connectionDialog.editTitle
              : t.tools.database.connectionDialog.addTitle}
          </DialogTitle>
          <DialogDescription>
            {connection
              ? t.tools.database.connectionDialog.editDescription
              : t.tools.database.connectionDialog.addDescription}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="scheme" className="text-right">
              {t.tools.database.connectionDialog.dbTypeLabel}
            </Label>
            <Select value={formData.scheme || 'mysql'} onValueChange={handleSchemeChange}>
              <SelectTrigger className="col-span-3">
                <div className="flex items-center gap-2">
                  {formData.scheme && (
                    <DatabaseIconComponent scheme={formData.scheme} size={18} />
                  )}
                  <SelectValue placeholder={t.tools.database.connectionDialog.dbTypePlaceholder}>
                    {formData.scheme && getDbConfig(formData.scheme).label}
                  </SelectValue>
                </div>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="mysql">
                  <div className="flex items-center gap-2.5">
                    <DatabaseIconComponent scheme="mysql" size={18} />
                    <span>{getDbConfig('mysql').label}</span>
                  </div>
                </SelectItem>
                <SelectItem value="postgres">
                  <div className="flex items-center gap-2.5">
                    <DatabaseIconComponent scheme="postgres" size={18} />
                    <span>{getDbConfig('postgres').label}</span>
                  </div>
                </SelectItem>
                <SelectItem value="mssql">
                  <div className="flex items-center gap-2.5">
                    <DatabaseIconComponent scheme="mssql" size={18} />
                    <span>{getDbConfig('mssql').label}</span>
                  </div>
                </SelectItem>
                <SelectItem value="oracle">
                  <div className="flex items-center gap-2.5">
                    <DatabaseIconComponent scheme="oracle" size={18} />
                    <span>{getDbConfig('oracle').label}</span>
                  </div>
                </SelectItem>
                <SelectItem value="mongodb">
                  <div className="flex items-center gap-2.5">
                    <DatabaseIconComponent scheme="mongodb" size={18} />
                    <span>{getDbConfig('mongodb').label}</span>
                  </div>
                </SelectItem>
                <SelectItem value="redis">
                  <div className="flex items-center gap-2.5">
                    <DatabaseIconComponent scheme="redis" size={18} />
                    <span>{getDbConfig('redis').label}</span>
                  </div>
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="host" className="text-right">
              {t.tools.database.connectionDialog.hostLabel}
            </Label>
            <Input
              id="host"
              value={formData.host || ''}
              onChange={(e) => setFormData({ ...formData, host: e.target.value })}
              className="col-span-3"
              placeholder={t.tools.database.connectionDialog.hostPlaceholder}
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="port" className="text-right">
              {t.tools.database.connectionDialog.portLabel}
            </Label>
            <Input
              id="port"
              type="number"
              value={formData.port || 3306}
              onChange={(e) =>
                setFormData({ ...formData, port: parseInt(e.target.value) || 3306 })
              }
              className="col-span-3"
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="username" className="text-right">
              {['mongodb', 'redis'].includes((formData.scheme || '').toLowerCase())
                ? t.tools.database.connectionDialog.usernameOptionalLabel
                : t.tools.database.connectionDialog.usernameLabel}
            </Label>
            <Input
              id="username"
              value={formData.username || ''}
              onChange={(e) => setFormData({ ...formData, username: e.target.value })}
              className="col-span-3"
              placeholder={
                ['mongodb', 'redis'].includes((formData.scheme || '').toLowerCase())
                  ? t.tools.database.connectionDialog.usernameOptionalPlaceholder
                  : ''
              }
            />
          </div>
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="password" className="text-right">
              {t.tools.database.connectionDialog.passwordLabel}
            </Label>
            <PasswordInput
              id="password"
              value={formData.password || ''}
              onChange={(e) => setFormData({ ...formData, password: e.target.value })}
              className="col-span-3"
            />
          </div>
          {formData.scheme === 'oracle' && (
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="serverName" className="text-right">
                {t.tools.database.connectionDialog.serviceNameLabel}
              </Label>
              <Input
                id="serverName"
                value={formData.serverName || ''}
                onChange={(e) => setFormData({ ...formData, serverName: e.target.value })}
                className="col-span-3"
              />
            </div>
          )}
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="notes" className="text-right">
              {t.tools.database.connectionDialog.notesLabel}
            </Label>
            <Textarea
              id="notes"
              value={formData.notes || ''}
              onChange={(e) => setFormData({ ...formData, notes: e.target.value })}
              className="col-span-3 min-h-[80px]"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t.common.cancel}
          </Button>
          <Button onClick={handleSave} disabled={loading}>
            <Save className="w-4 h-4 mr-2" />
            {t.common.save}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
