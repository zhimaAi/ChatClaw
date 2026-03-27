const RUN_STATUS_SUCCESS = 'success'
const RUN_STATUS_FAILED = 'failed'
const RUN_STATUS_ERROR = 'error'
const RUN_STATUS_OK = 'ok'
const RUN_STATUS_DELIVERED = 'delivered'

interface OpenClawLastRunStateInput {
  lastStatus?: string | null
  lastError?: string | null
}

function normalizeStatusValue(status?: string | null) {
  return String(status || '')
    .trim()
    .toLowerCase()
}

// normalizeOpenClawRunStatus keeps OpenClaw-native status variants mapped to UI badge states.
export function normalizeOpenClawRunStatus(status?: string | null) {
  const normalizedStatus = normalizeStatusValue(status)
  if (
    normalizedStatus === RUN_STATUS_SUCCESS ||
    normalizedStatus === RUN_STATUS_OK ||
    normalizedStatus === RUN_STATUS_DELIVERED
  ) {
    return RUN_STATUS_SUCCESS
  }
  if (
    normalizedStatus === RUN_STATUS_FAILED ||
    normalizedStatus === RUN_STATUS_ERROR
  ) {
    return RUN_STATUS_FAILED
  }
  return RUN_STATUS_SUCCESS
}

// getLastRunVisualState makes list-row status resilient when the backend only persisted last_error.
export function getLastRunVisualState(input: OpenClawLastRunStateInput) {
  const rawStatus = normalizeStatusValue(input.lastStatus)
  const normalizedStatus = normalizeOpenClawRunStatus(input.lastStatus)
  if (normalizedStatus === RUN_STATUS_FAILED) {
    return RUN_STATUS_FAILED
  }

  if (!rawStatus && String(input.lastError || '').trim()) {
    return RUN_STATUS_FAILED
  }

  return RUN_STATUS_SUCCESS
}
