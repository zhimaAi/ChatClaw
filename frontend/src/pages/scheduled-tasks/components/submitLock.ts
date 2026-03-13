export interface SubmitLock {
  acquire(externalSaving: boolean): boolean
  isLocked(): boolean
  reset(): void
  syncSaving(externalSaving: boolean): void
}

export function createSubmitLock(): SubmitLock {
  let locked = false

  return {
    acquire(externalSaving) {
      if (externalSaving || locked) {
        return false
      }

      locked = true
      return true
    },
    isLocked() {
      return locked
    },
    reset() {
      locked = false
    },
    syncSaving(externalSaving) {
      if (!externalSaving) {
        locked = false
      }
    },
  }
}
