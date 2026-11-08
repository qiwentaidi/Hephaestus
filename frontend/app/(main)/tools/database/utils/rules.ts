'use client'

import { fetchWithAuth } from '@/lib/api'
import type { DatabaseCollectRule } from '../types'

export async function loadCustomCollectRules(): Promise<DatabaseCollectRule[]> {
  const response = await fetchWithAuth('/api/v1/database/collect/rules')
  const data = await response.json()

  if (!response.ok || !data.success) {
    throw new Error(data.error || 'Failed to load custom collect rules')
  }

  return Array.isArray(data.data) ? data.data : []
}

export async function saveCustomCollectRules(rules: DatabaseCollectRule[]): Promise<void> {
  const response = await fetchWithAuth('/api/v1/database/collect/rules', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ rules }),
  })
  const data = await response.json()

  if (!response.ok || !data.success) {
    throw new Error(data.error || 'Failed to save custom collect rules')
  }
}
