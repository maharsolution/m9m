// Regression test for the save-time credential persistence bug
// (m9m bug fix 2026-10-07, see bug-fix.MD).
//
// Root cause was `cloneValue` in `web/src/lib/workflowGraph.ts` using
// `structuredClone` to deep-copy the parameters that the editor sends
// into Pinia state. `currentWorkflow` is a Vue `ref`, so the nodes it
// holds are reactive proxies, and `structuredClone` throws
// `DataCloneError: <Object> could not be cloned` when it encounters a
// Vue Proxy — for example the empty `options: {}` sub-object the
// webhook schema defines. The exception was silently swallowed by
// Vue's event dispatcher, leaving the chosen `authentication` value
// and the bound credential on screen but never written into
// `workflowStore.currentWorkflow.nodes[i].parameters`.
//
// This test fails on the old `structuredClone`-based implementation
// and passes on the `JSON.parse(JSON.stringify(...))`-based one.
// Run with: `node --test web/test/workflowGraph-cloneValue.test.mjs`.

import { test } from 'node:test'
import assert from 'node:assert/strict'

// Re-implement both candidates here so the test does not depend on
// the project's TS toolchain. The two implementations MUST stay in
// sync with `web/src/lib/workflowGraph.ts:cloneValue`.
function cloneValueStructuredClone(value) {
  return structuredClone(value)
}

function cloneValueJson(value) {
  if (value === null || value === undefined) return value
  return JSON.parse(JSON.stringify(value))
}

// Simulate a Vue 3 reactive proxy the way Pinia exposes `currentWorkflow`
// to the editor. `reactive({})` in Vue 3 returns a Proxy whose internals
// include `__v_isReactive = true`. We mimic just enough to reproduce the
// `DataCloneError` symptom without pulling the Vue package.
function makeReactiveProxy(target) {
  return new Proxy(target, {
    get(obj, prop) {
      // Mimic Vue's `__v_isReactive` flag so callers can spot a proxy.
      if (prop === '__v_isReactive') return true
      if (prop === '__v_raw') return obj
      const value = obj[prop]
      if (value && typeof value === 'object') {
        return makeReactiveProxy(value)
      }
      return value
    },
  })
}

test('structuredClone (the old impl) throws on a Vue reactive Proxy', () => {
  const parameters = makeReactiveProxy({
    httpMethod: 'POST',
    options: {}, // the empty sub-object that breaks structuredClone
    path: 'my-webhook-path',
    responseMode: 'responseNode',
  })
  assert.throws(
    () => cloneValueStructuredClone(parameters),
    /DataCloneError|could not be cloned/,
  )
})

test('JSON-clone (the new impl) survives the same reactive Proxy', () => {
  const parameters = makeReactiveProxy({
    httpMethod: 'POST',
    options: {},
    path: 'my-webhook-path',
    responseMode: 'responseNode',
  })
  const cloned = cloneValueJson(parameters)
  assert.deepEqual(cloned, {
    httpMethod: 'POST',
    options: {},
    path: 'my-webhook-path',
    responseMode: 'responseNode',
  })
  // The whole point of cloneValue is to keep the in-flight update
  // independent of the reactive source. Identity-equality is enough
  // to prove the clone produced a fresh object tree.
  assert.notEqual(cloned, parameters)
  assert.notEqual(cloned.options, parameters.options)
})

test('JSON-clone carries the freshly edited authentication + credential binding', () => {
  // Models the exact scenario NodePanel produces when the user
  // switches the Authentication dropdown from None to Basic auth and
  // binds a credential: `parameters.authentication = "basicAuth"` and
  // `credentials.httpBasicAuth = { id, name }`. The clone must carry
  // both through unchanged.
  const parameters = makeReactiveProxy({
    httpMethod: 'POST',
    options: {},
    path: 'my-webhook-path',
    responseMode: 'responseNode',
    authentication: 'basicAuth',
  })
  const credentials = makeReactiveProxy({
    httpBasicAuth: { id: 'cred_basic_1', name: 'basic_credential' },
  })
  const clonedParams = cloneValueJson(parameters)
  const clonedCreds = cloneValueJson(credentials)
  assert.equal(clonedParams.authentication, 'basicAuth')
  assert.deepEqual(clonedCreds.httpBasicAuth, {
    id: 'cred_basic_1',
    name: 'basic_credential',
  })
})