import { getLastRunVisualState, normalizeOpenClawRunStatus } from './status'

const FAILED_STATUS = 'failed'
const ERROR_STATUS = 'error'
const SUCCESS_STATUS = 'success'
const UNKNOWN_STATUS = 'queued'
const SAMPLE_ERROR = 'gateway timeout'

function assertEqual(actual: string, expected: string) {
  if (actual !== expected) {
    throw new Error(`expected "${expected}", got "${actual}"`)
  }
}

assertEqual(normalizeOpenClawRunStatus(FAILED_STATUS), FAILED_STATUS)
assertEqual(normalizeOpenClawRunStatus(ERROR_STATUS), FAILED_STATUS)
assertEqual(normalizeOpenClawRunStatus(SUCCESS_STATUS), SUCCESS_STATUS)
assertEqual(normalizeOpenClawRunStatus(UNKNOWN_STATUS), SUCCESS_STATUS)

assertEqual(
  getLastRunVisualState({
    lastStatus: ERROR_STATUS,
    lastError: '',
  }),
  FAILED_STATUS
)

assertEqual(
  getLastRunVisualState({
    lastStatus: '',
    lastError: SAMPLE_ERROR,
  }),
  FAILED_STATUS
)

assertEqual(
  getLastRunVisualState({
    lastStatus: SUCCESS_STATUS,
    lastError: SAMPLE_ERROR,
  }),
  SUCCESS_STATUS
)
