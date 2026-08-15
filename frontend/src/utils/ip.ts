import type { SpokeInfo } from '../types'

/**
 * Compare two IP addresses in ascending numerical order.
 * Handles IPv4 (with optional CIDR / port) and IPv6 gracefully.
 */
export function compareIP(a?: string, b?: string): number {
  if (!a && !b) return 0
  if (!a) return 1
  if (!b) return -1

  const cleanA = a.split('/')[0].split(':')[0].trim()
  const cleanB = b.split('/')[0].split(':')[0].trim()

  const partsA = cleanA.split('.').map(Number)
  const partsB = cleanB.split('.').map(Number)

  if (partsA.length === 4 && partsB.length === 4 && !partsA.some(isNaN) && !partsB.some(isNaN)) {
    for (let i = 0; i < 4; i++) {
      if (partsA[i] !== partsB[i]) {
        return partsA[i] - partsB[i]
      }
    }
    return 0
  }

  return cleanA.localeCompare(cleanB, undefined, { numeric: true, sensitivity: 'base' })
}

/**
 * Sort an array of SpokeInfo records by their protocol_address in ascending order.
 */
export function sortSpokesByIP(spokes: SpokeInfo[]): SpokeInfo[] {
  if (!spokes || !Array.isArray(spokes)) return []
  return [...spokes].sort((a, b) => compareIP(a.protocol_address, b.protocol_address))
}
