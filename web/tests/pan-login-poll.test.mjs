import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  PAN_LOGIN_MAX_FAILURES,
  PAN_LOGIN_POLL_MS,
  panLoginPollDelay
} from '../src/lib/pan-login.ts'

for (const scenario of [
  {
    name: 'a waiting login keeps polling',
    input: { failed: false, failureCount: 0, state: 'waiting' },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'a scanned login keeps polling until the phone confirms',
    input: { failed: false, failureCount: 0, state: 'scanned' },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'the first answer has not arrived yet',
    input: { failed: false, failureCount: 0, state: undefined },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'a completed login stops polling',
    input: { failed: false, failureCount: 0, state: 'authorized' },
    want: false
  },
  {
    name: 'an expired login stops polling so the dialog can refresh it',
    input: { failed: false, failureCount: 0, state: 'expired' },
    want: false
  },
  {
    name: 'a login the user canceled on their phone stops polling',
    input: { failed: false, failureCount: 0, state: 'canceled' },
    want: false
  },
  {
    name: 'a transport failure keeps the QR code the user is aiming at',
    input: { failed: true, failureCount: 1, state: 'waiting' },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'a failure after the user scanned keeps that state on screen',
    input: { failed: true, failureCount: 1, state: 'scanned' },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'the last tolerated failure still retries',
    input: { failed: true, failureCount: PAN_LOGIN_MAX_FAILURES - 1, state: 'waiting' },
    want: PAN_LOGIN_POLL_MS
  },
  {
    name: 'a run of failures long enough to mean the backend is unreachable',
    input: { failed: true, failureCount: PAN_LOGIN_MAX_FAILURES, state: 'waiting' },
    want: false
  },
  {
    name: 'failures past the budget stay stopped',
    input: { failed: true, failureCount: PAN_LOGIN_MAX_FAILURES + 3, state: undefined },
    want: false
  }
]) {
  test(scenario.name, () => {
    assert.equal(panLoginPollDelay(scenario.input), scenario.want)
  })
}
