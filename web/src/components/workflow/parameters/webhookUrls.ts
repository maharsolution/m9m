/**
 * Webhook URL helpers for the Webhook node's parameter panel.
 *
 * n8n exposes TWO URLs for every Webhook node:
 *
 *   - Production URL — `<base>/webhook/<path>`; it only fires while the
 *     workflow is ACTIVE.
 *   - Test URL       — `<base>/webhook-test/<path>`; it fires even while
 *     the workflow is still being designed (that is how "Test workflow"
 *     captures the input / output of a flow).
 *
 * The public base is the origin the editor itself is served from — the
 * editor and the API live on the same host, so `window.location.origin`
 * is the single source of truth. The helpers below are pure so they can
 * be unit-tested without a DOM.
 */

/** Path prefix of the production webhook endpoint. */
export const WEBHOOK_PRODUCTION_PREFIX = '/webhook'

/** Path prefix of the n8n-compatible test webhook endpoint. */
export const WEBHOOK_TEST_PREFIX = '/webhook-test'

/**
 * Placeholder used when the Webhook node has no path yet. n8n generates
 * a random path on node creation; m9m keeps the field empty until the
 * user types one, so the panel shows a readable placeholder instead of a
 * bare trailing slash.
 */
export const WEBHOOK_PATH_PLACEHOLDER = 'your-path'

export interface WebhookUrls {
  /** `<base>/webhook/<path>` — only live while the workflow is active. */
  productionUrl: string
  /** `<base>/webhook-test/<path>` — live while the flow is being designed. */
  testUrl: string
}

/**
 * Reports whether a node type is the Webhook trigger. n8n's canonical
 * type id is `n8n-nodes-base.webhook`; m9m also accepts the
 * `n8n-nodes-base.webhook` variants shipped by forks, so the check is
 * a case-insensitive substring match (same rule the backend uses in
 * `internal/webhooks/manager.go`).
 */
export function isWebhookNodeType(nodeType: string | null | undefined): boolean {
  if (!nodeType) return false
  return nodeType.toLowerCase().includes('webhook')
}

/**
 * Normalises a webhook path: leading / trailing slashes are removed so
 * the path can be joined to the endpoint prefix without producing a
 * double slash.
 */
export function normalizeWebhookPath(path: string | null | undefined): string {
  if (!path) return ''
  return path.trim().replace(/^\/+/, '').replace(/\/+$/, '')
}

/**
 * Builds the production and test URLs of a Webhook node.
 *
 * `baseUrl` is the public base (e.g. `https://m9m.example.com`). When the
 * node has no path yet, both URLs carry a placeholder path so the user
 * still sees where the endpoints will live.
 */
export function buildWebhookUrls(baseUrl: string, path: string | null | undefined): WebhookUrls {
  const base = (baseUrl ?? '').trim().replace(/\/+$/, '')
  const segment = normalizeWebhookPath(path) || WEBHOOK_PATH_PLACEHOLDER

  return {
    productionUrl: `${base}${WEBHOOK_PRODUCTION_PREFIX}/${segment}`,
    testUrl: `${base}${WEBHOOK_TEST_PREFIX}/${segment}`,
  }
}

/**
 * The public base of the current page — `window.location.origin`.
 *
 * Falls back to an empty string outside a browser (unit tests, SSR) so
 * the caller can decide what to render.
 */
export function currentPublicBase(): string {
  if (typeof window === 'undefined') return ''
  return window.location?.origin ?? ''
}
