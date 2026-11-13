import axios from 'axios'
import apiClient from '@/api/client'
import type { WebSocketMessage, WebSocketMessageType } from '@/types/api'

type MessageHandler = (message: WebSocketMessage) => void

/**
 * Connection state of the push channel.
 *
 * - `connected`       the handshake succeeded, the stream is live
 * - `connecting`      a handshake attempt is running (or a retry is pending)
 * - `unauthenticated` there is no session at all - nobody signed in
 * - `expired`         the session token was rejected (missing or expired)
 * - `disconnected`    the backend or the stream is unreachable
 */
export type ConnectionStatus =
  | 'connected'
  | 'connecting'
  | 'unauthenticated'
  | 'expired'
  | 'disconnected'

type StatusHandler = (status: ConnectionStatus) => void

interface WebSocketOptions {
  url?: string
  /** Base delay of the reconnect backoff, in milliseconds. */
  reconnectInterval?: number
  /** Upper bound of the reconnect backoff, in milliseconds. */
  maxReconnectDelay?: number
  /** Maximum number of reconnect attempts; 0 means retry forever. */
  maxReconnectAttempts?: number
}

interface PushTicketResponse {
  ticket: string
  expiresIn: number
}

class WebSocketConnection {
  private ws: WebSocket | null = null
  private url: string
  private reconnectInterval: number
  private maxReconnectDelay: number
  private maxReconnectAttempts: number
  private reconnectAttempts = 0
  private handlers: Map<WebSocketMessageType | '*', Set<MessageHandler>> = new Map()
  private statusHandlers: Set<StatusHandler> = new Set()
  private isConnecting = false
  private shouldReconnect = true
  private status: ConnectionStatus = 'disconnected'
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null

  constructor(options: WebSocketOptions = {}) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    this.url = options.url || `${protocol}//${host}/api/v1/push`
    this.reconnectInterval = options.reconnectInterval || 1000
    this.maxReconnectDelay = options.maxReconnectDelay || 15000
    // 0 = retry forever: a server restart must recover on its own,
    // without a page reload.
    this.maxReconnectAttempts = options.maxReconnectAttempts || 0
  }

  /**
   * Opens the push channel.
   *
   * The browser WebSocket API cannot send an Authorization header, so
   * the session token is exchanged for a one-time ticket first
   * (POST /api/v1/push/ticket - an ordinary, authenticated API call).
   * The ticket travels in the query string of the handshake and
   * authorises exactly one connection.
   */
  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN || this.isConnecting) {
      return
    }

    this.isConnecting = true
    this.shouldReconnect = true
    this.setStatus('connecting')

    void this.connectWithTicket()
  }

  private async connectWithTicket(): Promise<void> {
    // No session at all: nobody signed in. There is nothing to
    // authenticate the handshake with.
    if (!localStorage.getItem('auth_token')) {
      this.isConnecting = false
      this.setStatus('unauthenticated')
      return
    }

    let ticket: string
    try {
      const response = await apiClient.post<PushTicketResponse>('/push/ticket')
      ticket = response.data.ticket
    } catch (error) {
      this.isConnecting = false

      const status = axios.isAxiosError(error) ? error.response?.status : undefined
      if (status === 401 || status === 403) {
        // The session token is missing, expired or rejected: that is an
        // auth problem, not a disconnected backend.
        this.setStatus('expired')
        return
      }

      // The backend could not be reached (or failed): the stream is
      // genuinely down. The retry loop takes care of it.
      console.error('[WebSocket] Ticket request failed:', error)
      this.setStatus('disconnected')
      this.scheduleReconnect()
      return
    }

    if (!this.shouldReconnect) {
      this.isConnecting = false
      return
    }

    try {
      this.ws = new WebSocket(`${this.url}?ticket=${encodeURIComponent(ticket)}`)

      this.ws.onopen = () => {
        console.log('[WebSocket] Connected')
        this.isConnecting = false
        this.reconnectAttempts = 0
        this.setStatus('connected')
      }

      this.ws.onmessage = (event) => {
        try {
          const message: WebSocketMessage = JSON.parse(event.data)
          this.emit(message)
        } catch (error) {
          console.error('[WebSocket] Failed to parse message:', error)
        }
      }

      this.ws.onclose = (event) => {
        console.log('[WebSocket] Disconnected:', event.code, event.reason)
        this.isConnecting = false
        this.ws = null

        if (this.shouldReconnect) {
          // A closed stream is being re-established, not lost.
          this.setStatus('connecting')
          this.scheduleReconnect()
        }
      }

      this.ws.onerror = (error) => {
        console.error('[WebSocket] Error:', error)
        this.isConnecting = false
      }
    } catch (error) {
      console.error('[WebSocket] Connection error:', error)
      this.isConnecting = false
      this.setStatus('disconnected')
      this.scheduleReconnect()
    }
  }

  /**
   * Closes the push channel.
   *
   * The status lets the caller say WHY the channel was closed: a sign
   * out is an auth state, not a disconnected server.
   */
  disconnect(status: ConnectionStatus = 'disconnected'): void {
    this.shouldReconnect = false
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.isConnecting = false
    this.setStatus(status)
  }

  private scheduleReconnect(): void {
    if (!this.shouldReconnect) return
    if (this.reconnectTimer) return

    this.reconnectAttempts++
    if (this.maxReconnectAttempts > 0 && this.reconnectAttempts > this.maxReconnectAttempts) {
      console.warn('[WebSocket] Reconnect limit reached - giving up')
      return
    }

    // Exponential backoff, capped: a server that comes back after a
    // restart is picked up again without hammering it.
    const delay = Math.min(
      this.reconnectInterval * Math.pow(2, this.reconnectAttempts - 1),
      this.maxReconnectDelay
    )
    console.log(`[WebSocket] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`)

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      if (this.shouldReconnect) {
        this.connect()
      }
    }, delay)
  }

  send(data: Record<string, unknown>): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data))
    } else {
      console.warn('[WebSocket] Cannot send - not connected')
    }
  }

  on(type: WebSocketMessageType | '*', handler: MessageHandler): () => void {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, new Set())
    }
    this.handlers.get(type)!.add(handler)

    // Return unsubscribe function
    return () => {
      this.handlers.get(type)?.delete(handler)
    }
  }

  off(type: WebSocketMessageType | '*', handler: MessageHandler): void {
    this.handlers.get(type)?.delete(handler)
  }

  /**
   * Subscribes to the connection state. Returns an unsubscribe
   * function.
   */
  onStatus(handler: StatusHandler): () => void {
    this.statusHandlers.add(handler)
    return () => {
      this.statusHandlers.delete(handler)
    }
  }

  private setStatus(status: ConnectionStatus): void {
    if (this.status === status) return
    this.status = status
    this.statusHandlers.forEach((handler) => {
      try {
        handler(status)
      } catch (error) {
        console.error('[WebSocket] Status handler error:', error)
      }
    })
  }

  private emit(message: WebSocketMessage): void {
    // Call specific type handlers
    this.handlers.get(message.type)?.forEach((handler) => {
      try {
        handler(message)
      } catch (error) {
        console.error('[WebSocket] Handler error:', error)
      }
    })

    // Call wildcard handlers
    this.handlers.get('*')?.forEach((handler) => {
      try {
        handler(message)
      } catch (error) {
        console.error('[WebSocket] Handler error:', error)
      }
    })
  }

  get isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN
  }

  get connectionState(): ConnectionStatus {
    return this.status
  }
}

// Singleton instance
export const wsConnection = new WebSocketConnection()

export default WebSocketConnection
