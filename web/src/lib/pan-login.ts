import type { PanLoginState } from '@/api/pan'

export const PAN_LOGIN_POLL_MS = 1500

// A login survives a burst of transport failures: the QR code on screen is still
// the one the user is aiming at, so dropping it costs them a rescan. Only a run of
// failures long enough to mean the backend is unreachable hands control back.
//
// Only a real transport error counts against the budget: the backend answers a
// long poll that brings no news with `waiting` rather than an error.
export const PAN_LOGIN_MAX_FAILURES = 5

// The budget lives in react-query's retry loop, which keeps the previous answer on
// screen while it runs and only reports an error once the budget is spent.
// react-query asks after each failure, passing the failures before it, so the
// attempt that just failed is number failureCount + 1.
export function panLoginShouldRetry(failureCount: number): boolean {
  return failureCount + 1 < PAN_LOGIN_MAX_FAILURES
}

// Polling continues between answers. A failing poll retries inside its own fetch
// on the budget above; once that is spent the dialog takes over, so an error ends
// the polling.
export function panLoginPollDelay(input: {
  failed: boolean
  state?: PanLoginState
}): number | false {
  if (input.failed) return false
  const state = input.state
  return !state || state === 'waiting' || state === 'scanned' ? PAN_LOGIN_POLL_MS : false
}
