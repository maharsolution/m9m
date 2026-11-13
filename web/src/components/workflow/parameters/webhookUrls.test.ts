import { describe, expect, it } from 'vitest'
import {
  buildWebhookUrls,
  isWebhookNodeType,
  normalizeWebhookPath,
  WEBHOOK_PATH_PLACEHOLDER,
} from './webhookUrls'

describe('webhook URL helpers', () => {
  it('recognises the webhook node type', () => {
    expect(isWebhookNodeType('n8n-nodes-base.webhook')).toBe(true)
    expect(isWebhookNodeType('n8n-nodes-base.Webhook')).toBe(true)
    expect(isWebhookNodeType('n8n-nodes-base.set')).toBe(false)
    expect(isWebhookNodeType('')).toBe(false)
    expect(isWebhookNodeType(undefined)).toBe(false)
  })

  it('normalises the webhook path', () => {
    expect(normalizeWebhookPath('webhook_xml')).toBe('webhook_xml')
    expect(normalizeWebhookPath('/webhook_xml')).toBe('webhook_xml')
    expect(normalizeWebhookPath('/webhook_xml/')).toBe('webhook_xml')
    expect(normalizeWebhookPath('  ')).toBe('')
    expect(normalizeWebhookPath(undefined)).toBe('')
  })

  it('builds both n8n-style URLs from the public base', () => {
    const urls = buildWebhookUrls('http://localhost:3000', 'webhook_xml')
    expect(urls.productionUrl).toBe('http://localhost:3000/webhook/webhook_xml')
    expect(urls.testUrl).toBe('http://localhost:3000/webhook-test/webhook_xml')
  })

  it('strips a trailing slash from the public base', () => {
    const urls = buildWebhookUrls('https://m9m.example.com/', 'hook')
    expect(urls.productionUrl).toBe('https://m9m.example.com/webhook/hook')
    expect(urls.testUrl).toBe('https://m9m.example.com/webhook-test/hook')
  })

  it('falls back to a placeholder path when the node has none', () => {
    const urls = buildWebhookUrls('http://localhost:3000', '')
    expect(urls.productionUrl).toBe(
      `http://localhost:3000/webhook/${WEBHOOK_PATH_PLACEHOLDER}`,
    )
    expect(urls.testUrl).toBe(
      `http://localhost:3000/webhook-test/${WEBHOOK_PATH_PLACEHOLDER}`,
    )
  })

  it('keeps the base untouched when it is empty', () => {
    const urls = buildWebhookUrls('', 'hook')
    expect(urls.productionUrl).toBe('/webhook/hook')
    expect(urls.testUrl).toBe('/webhook-test/hook')
  })
})
