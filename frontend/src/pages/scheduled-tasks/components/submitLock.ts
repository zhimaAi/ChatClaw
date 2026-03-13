export interface SubmitLock {
  acquire(externalSaving: boolean): boolean
  isLocked(): boolean
  reset(): void
  syncSaving(externalSaving: boolean): void
}

const MIN_SUBMIT_LOCK_MS = 1000

export function createSubmitLock(minLockMs = MIN_SUBMIT_LOCK_MS): SubmitLock {
  let locked = false
  let releaseAt = 0
  let releaseTimer: ReturnType<typeof setTimeout> | null = null

  function clearReleaseTimer() {
    if (releaseTimer) {
      clearTimeout(releaseTimer)
      releaseTimer = null
    }
  }

  function unlock() {
    clearReleaseTimer()
    locked = false
    releaseAt = 0
  }

  function scheduleRelease() {
    clearReleaseTimer()
    const remainingMs = releaseAt - Date.now()
    if (remainingMs <= 0) {
      unlock()
      return
    }
    releaseTimer = setTimeout(() => {
      unlock()
    }, remainingMs)
  }

  return {
    acquire(externalSaving) {
      if (externalSaving || locked) {
        return false
      }

      locked = true
      releaseAt = Date.now() + minLockMs
      return true
    },
    isLocked() {
      return locked
    },
    reset() {
      unlock()
    },
    syncSaving(externalSaving) {
      if (!externalSaving) {
        scheduleRelease()
      }
    },
  }
}
