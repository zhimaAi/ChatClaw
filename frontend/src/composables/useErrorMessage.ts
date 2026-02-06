export function getErrorMessage(error: unknown): string {
  let msg = ''
  if (error instanceof Error) msg = error.message
  else if (typeof error === 'string') msg = error
  else if (typeof error === 'object' && error !== null && 'message' in error) {
    msg = String((error as { message: unknown }).message)
  } else msg = String(error)

  // Wails / bindings sometimes return placeholder strings like "<no value>".
  // Treat these as empty so callers can fall back to i18n messages.
  const normalized = msg.trim()
  if (
    normalized === '' ||
    normalized === '<no value>' ||
    normalized === 'undefined' ||
    normalized === 'null'
  ) {
    return ''
  }

  if (msg.startsWith('{')) {
    try {
      const parsed = JSON.parse(msg)
      if (parsed?.message) return String(parsed.message)
    } catch {
      // ignore
    }
  }
  return msg
}
