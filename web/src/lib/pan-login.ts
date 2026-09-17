import type { PanLoginState } from '@/api/pan'

export const PAN_LOGIN_POLL_MS = 1500

// A login survives a burst of transport failures: the QR code on screen is still
// the one the user is aiming at, so dropping it costs them a rescan. Only a run of
// failures long enough to mean the backend is unreachable hands control back.
//
// Only a real transport error counts against the budget: the backend answers a
// long poll that brings no news with `waiting` rather than an error.
export const PAN_LOGIN_MAX_FAILURES = 5

export function panLoginPollDelay(input: {
  failed: boolean
  failureCount: number
  state?: PanLoginState
}): number | false {
  if (input.failed) {
    return input.failureCount < PAN_LOGIN_MAX_FAILURES ? PAN_LOGIN_POLL_MS : false
  }
  const state = input.state
  return !state || state === 'waiting' || state === 'scanned' ? PAN_LOGIN_POLL_MS : false
}
