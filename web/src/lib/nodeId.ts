/**
 * Node id helper.
 *
 * `crypto.randomUUID` is unavailable on insecure origins (the editor
 * is often served over plain http), so a timestamp + counter
 * fallback is used there. The id has to be unique across the whole
 * workflow — it is the key of the node list *and* of the Vue Flow
 * node map.
 */

let nodeIdCounter = 0

export function generateNodeId(): string {
  const cryptoApi = globalThis.crypto
  if (cryptoApi && typeof cryptoApi.randomUUID === 'function') {
    return cryptoApi.randomUUID()
  }
  nodeIdCounter += 1
  return `node_${Date.now().toString(36)}_${nodeIdCounter.toString(36)}`
}
