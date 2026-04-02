/**
 * Format integral/credits stats: round to one decimal; omit the fraction when it is .0.
 */
export function formatChatwikiIntegralNumber(num: number): string {
  if (!Number.isFinite(num)) return String(num)
  const roundedTenth = Math.round(num * 10) / 10
  const fractionIsZero = Math.abs(roundedTenth - Math.round(roundedTenth)) < 1e-9
  if (fractionIsZero) {
    return Math.round(roundedTenth).toLocaleString(undefined, { maximumFractionDigits: 0 })
  }
  return roundedTenth.toLocaleString(undefined, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  })
}
