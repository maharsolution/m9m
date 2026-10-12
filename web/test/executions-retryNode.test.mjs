// Regression test for the "Retry from here" button wiring
// (m9m bug fix 2026-10-12, BUG-UI-2026-09-09-002).
//
// The retry-from-here button in `web/src/views/ExecutionDetail.vue`
// (template lines 1009-1021, handler `retryFromSelected` lines
// 797-810) calls `executionStore.retryNode(executionId, {
// nodeName })` which delegates to `api/executions.ts::retryNode`,
// which POSTs `/api/v1/executions/{id}/retry-node` with the body
// `{ nodeName, inputData?, mode? }`.
//
// This test exercises the API client path against a mock
// `apiClient.post` so a future refactor (e.g. moving the call to
// TanStack Query, or adding a new field) can't silently break the
// button wiring.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// Mock the axios-style client the same way the rest of the suite
// does. We want to verify the URL path + request body + return
// shape; everything else is plumbing.
const post = vi.fn()
const get = vi.fn()
const del = vi.fn()
vi.mock('../src/api/client', () => ({
  default: {
    post,
    get,
    delete: del,
  },
}))

describe('executions.retryNode — "Retry from here" button wiring', () => {
  beforeEach(() => {
    post.mockReset()
    get.mockReset()
    del.mockReset()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('POSTs to /executions/{id}/retry-node with the node name', async () => {
    // The store / view passes the result of `getExecution` and
    // the user-selected node name. The wire shape MUST stay
    // {nodeName} — the backend reads `req.NodeName` verbatim.
    const { retryNode } = await import('../src/api/executions')
    const fakeExecution = {
      id: 'new-exec-123',
      workflowId: 'wf-1',
      mode: 'retry-node',
      status: 'running',
    }
    post.mockResolvedValue({ data: fakeExecution })

    const result = await retryNode('exec-abc', { nodeName: 'Mappin Customer CFSP' })

    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith('/executions/exec-abc/retry-node', {
      nodeName: 'Mappin Customer CFSP',
    })
    expect(result).toEqual(fakeExecution)
  })

  it('forwards an optional inputData override (Run with edited input)', async () => {
    const { retryNode } = await import('../src/api/executions')
    post.mockResolvedValue({ data: { id: 'new-1' } })

    await retryNode('exec-1', {
      nodeName: 'Mappin Customer CFSP',
      inputData: [{ json: { customer_id: 'CFSP-001' } }],
      mode: 'manual',
    })

    expect(post).toHaveBeenCalledWith('/executions/exec-1/retry-node', {
      nodeName: 'Mappin Customer CFSP',
      inputData: [{ json: { customer_id: 'CFSP-001' } }],
      mode: 'manual',
    })
  })

  it('propagates backend errors so the view can show a toast', async () => {
    const { retryNode } = await import('../src/api/executions')
    const err = new Error('500 from /retry-node')
    post.mockRejectedValue(err)

    await expect(
      retryNode('exec-1', { nodeName: 'Mappin Customer CFSP' })
    ).rejects.toThrow('500 from /retry-node')
  })

  it('uses the full URL relative to the API base (no manual prefix)', async () => {
    // Guard against a refactor that accidentally prepends
    // /api/v1/ at the call site — the apiClient already does it
    // via its baseURL config.
    const { retryNode } = await import('../src/api/executions')
    post.mockResolvedValue({ data: { id: 'x' } })

    await retryNode('exec-9', { nodeName: 'A' })

    const [path] = post.mock.calls[0]
    expect(path.startsWith('/api/')).toBe(false)
    expect(path).toBe('/executions/exec-9/retry-node')
  })
})
