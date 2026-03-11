/**
 * ChatWiki API result cache with 5-minute TTL.
 * - Read: prefer local cache; if cache is older than 5 min, return cache and refresh in background.
 * - Sync (account management): clearAll() then re-fetch.
 */
import {
  ChatWikiService,
  type Binding,
  type Library,
  type Robot,
} from '@bindings/chatclaw/internal/services/chatwiki'

const CACHE_TTL_MS = 5 * 60 * 1000

export interface CacheEntry<T> {
  data: T
  cachedAt: number
}

let bindingCacheRef: CacheEntry<Binding | null> | null = null
let robotListCache: CacheEntry<Robot[]> | null = null
let robotListAllCache: CacheEntry<Robot[]> | null = null
const libraryListCache = new Map<number, CacheEntry<Library[]>>()
const libraryListOnlyOpenCache = new Map<number, CacheEntry<Library[]>>()

function isStale(entry: CacheEntry<unknown>): boolean {
  return Date.now() - entry.cachedAt > CACHE_TTL_MS
}

/**
 * Get binding: return cache first; if cache older than 5 min, refresh in background.
 */
export async function getBinding(): Promise<Binding | null> {
  const now = Date.now()
  if (bindingCacheRef && !isStale(bindingCacheRef)) {
    return bindingCacheRef.data
  }
  if (bindingCacheRef && isStale(bindingCacheRef)) {
    const stale = bindingCacheRef.data
    ChatWikiService.GetBinding()
      .then((data) => {
        bindingCacheRef = { data: data ?? null, cachedAt: Date.now() }
      })
      .catch(() => {
        clearAll()
      })
    return stale
  }
  const data = await ChatWikiService.GetBinding()
  bindingCacheRef = { data: data ?? null, cachedAt: now }
  return data ?? null
}

/**
 * Get robot list (only open). Cached with same TTL and async refresh when stale.
 */
export async function getRobotList(): Promise<Robot[]> {
  const now = Date.now()
  if (robotListCache && !isStale(robotListCache)) {
    return robotListCache.data
  }
  if (robotListCache && isStale(robotListCache)) {
    const stale = robotListCache.data
    ChatWikiService.GetRobotList()
      .then((list) => {
        robotListCache = { data: list ?? [], cachedAt: Date.now() }
      })
      .catch(() => {
        clearAll()
      })
    return stale
  }
  const list = await ChatWikiService.GetRobotList()
  const data = list ?? []
  robotListCache = { data, cachedAt: now }
  return data
}

/**
 * Get all robot list. Cached with same TTL and async refresh when stale.
 */
export async function getRobotListAll(): Promise<Robot[]> {
  const now = Date.now()
  if (robotListAllCache && !isStale(robotListAllCache)) {
    return robotListAllCache.data
  }
  if (robotListAllCache && isStale(robotListAllCache)) {
    const stale = robotListAllCache.data
    ChatWikiService.GetRobotListAll()
      .then((list) => {
        robotListAllCache = { data: list ?? [], cachedAt: Date.now() }
      })
      .catch(() => {
        clearAll()
      })
    return stale
  }
  const list = await ChatWikiService.GetRobotListAll()
  const data = list ?? []
  robotListAllCache = { data, cachedAt: now }
  return data
}

/**
 * Get library list by libType. Cached per type with same TTL and async refresh when stale.
 */
export async function getLibraryList(libType: number): Promise<Library[]> {
  const now = Date.now()
  const entry = libraryListCache.get(libType)
  if (entry && !isStale(entry)) {
    return entry.data
  }
  if (entry && isStale(entry)) {
    const stale = entry.data
    ChatWikiService.GetLibraryList(libType)
      .then((list) => {
        libraryListCache.set(libType, { data: list ?? [], cachedAt: Date.now() })
      })
      .catch(() => {
        clearAll()
      })
    return stale
  }
  const list = await ChatWikiService.GetLibraryList(libType)
  const data = list ?? []
  libraryListCache.set(libType, { data, cachedAt: now })
  return data
}

/**
 * Get library list (only open) by libType. Cached per type with same TTL and async refresh when stale.
 */
export async function getLibraryListOnlyOpen(libType: number): Promise<Library[]> {
  const now = Date.now()
  const entry = libraryListOnlyOpenCache.get(libType)
  if (entry && !isStale(entry)) {
    return entry.data
  }
  if (entry && isStale(entry)) {
    const stale = entry.data
    ChatWikiService.GetLibraryListOnlyOpen(libType)
      .then((list) => {
        libraryListOnlyOpenCache.set(libType, {
          data: list ?? [],
          cachedAt: Date.now(),
        })
      })
      .catch(() => {
        clearAll()
      })
    return stale
  }
  const list = await ChatWikiService.GetLibraryListOnlyOpen(libType)
  const data = list ?? []
  libraryListOnlyOpenCache.set(libType, { data, cachedAt: now })
  return data
}

/**
 * Clear all ChatWiki caches. Call when user clicks "sync" in account management.
 */
export function clearAll(): void {
  bindingCacheRef = null
  robotListCache = null
  robotListAllCache = null
  libraryListCache.clear()
  libraryListOnlyOpenCache.clear()
}
