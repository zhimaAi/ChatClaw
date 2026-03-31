const SAFE_DATE_HOUR = 12
const SAFE_DATE_MINUTE = 0
const SAFE_DATE_SECOND = 0
const SAFE_DATE_MILLISECOND = 0
const MONTHS_PER_YEAR = 12
const YEAR_RANGE_OFFSET = 5
const MONTH_INDEX_OFFSET = 1
const FIRST_DAY_OF_MONTH = 1

function createSafeDate(year: number, month: number, day: number) {
  return new Date(
    year,
    month,
    day,
    SAFE_DATE_HOUR,
    SAFE_DATE_MINUTE,
    SAFE_DATE_SECOND,
    SAFE_DATE_MILLISECOND
  )
}

export function startOfExpirationMonth(date: Date) {
  return createSafeDate(date.getFullYear(), date.getMonth(), FIRST_DAY_OF_MONTH)
}

export function addExpirationMonths(date: Date, amount: number) {
  return createSafeDate(date.getFullYear(), date.getMonth() + amount, FIRST_DAY_OF_MONTH)
}

export function buildExpirationMonthOptions() {
  return Array.from({ length: MONTHS_PER_YEAR }, (_, index) => index + MONTH_INDEX_OFFSET)
}

export function buildExpirationYearOptions(visibleMonth: Date) {
  const currentYear = visibleMonth.getFullYear()
  const startYear = currentYear - YEAR_RANGE_OFFSET
  const endYear = currentYear + YEAR_RANGE_OFFSET
  return Array.from(
    { length: endYear - startYear + MONTH_INDEX_OFFSET },
    (_, index) => startYear + index
  )
}

export function setVisibleMonthYear(visibleMonth: Date, year?: number, month?: number) {
  const nextYear = year ?? visibleMonth.getFullYear()
  const nextMonth = (month ?? visibleMonth.getMonth() + MONTH_INDEX_OFFSET) - MONTH_INDEX_OFFSET
  return createSafeDate(nextYear, nextMonth, FIRST_DAY_OF_MONTH)
}
