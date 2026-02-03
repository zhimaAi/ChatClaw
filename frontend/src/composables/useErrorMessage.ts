export function getErrorMessage(error: unknown): string {
  let msg = ''
  if (error instanceof Error) msg = error.message
  else if (typeof error === 'string') msg = error
  else if (typeof error === 'object' && error !== null && 'message' in error) {
    msg = String((error as { message: unknown }).message)
  } else msg = String(error)

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
