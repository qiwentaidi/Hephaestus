const DB_NAME = 'hephaestus-ai-workbench'
const DB_VERSION = 1
const STORE_NAME = 'composer-queues'

let database: Promise<IDBDatabase | null> | null = null

function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof window === 'undefined' || typeof window.indexedDB === 'undefined') return Promise.resolve(null)
  if (database) return database
  database = new Promise<IDBDatabase | null>((resolve) => {
    let request: IDBOpenDBRequest
    try {
      request = window.indexedDB.open(DB_NAME, DB_VERSION)
    } catch {
      resolve(null)
      return
    }
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE_NAME)) db.createObjectStore(STORE_NAME)
    }
    request.onsuccess = () => {
      const db = request.result
      db.onversionchange = () => {
        db.close()
        database = null
      }
      resolve(db)
    }
    request.onerror = () => resolve(null)
    request.onblocked = () => resolve(null)
  })
  return database
}

/**
 * Queued messages belong to the signed-in account; scoping the store by the
 * current user avoids restoring another account's pending prompts.
 */
export function workbenchQueueOwnerKey(): string {
  if (typeof window === 'undefined') return 'anonymous'
  try {
    const raw = window.localStorage.getItem('userData')
    const user = raw ? JSON.parse(raw) : null
    const identity = user?.id ?? user?.username ?? user?.email
    return identity === undefined || identity === null || identity === '' ? 'anonymous' : String(identity)
  } catch {
    return 'anonymous'
  }
}

export async function loadQueuedMessages<T>(owner: string): Promise<T[]> {
  const db = await openDatabase()
  if (!db) return []
  return new Promise<T[]>((resolve) => {
    try {
      const transaction = db.transaction(STORE_NAME, 'readonly')
      const request = transaction.objectStore(STORE_NAME).get(owner)
      request.onsuccess = () => {
        const stored = request.result as { messages?: unknown } | undefined
        resolve(Array.isArray(stored?.messages) ? stored.messages as T[] : [])
      }
      request.onerror = () => resolve([])
    } catch {
      resolve([])
    }
  })
}

export async function saveQueuedMessages<T>(owner: string, messages: T[]): Promise<void> {
  const db = await openDatabase()
  if (!db) return
  return new Promise<void>((resolve) => {
    try {
      const transaction = db.transaction(STORE_NAME, 'readwrite')
      const store = transaction.objectStore(STORE_NAME)
      if (messages.length) store.put({ messages, savedAt: Date.now() }, owner)
      else store.delete(owner)
      transaction.oncomplete = () => resolve()
      transaction.onerror = () => resolve()
      transaction.onabort = () => resolve()
    } catch {
      resolve()
    }
  })
}
