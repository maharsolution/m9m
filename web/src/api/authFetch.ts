/**
 * Authenticated `fetch` for the few call sites that need raw fetch
 * semantics (status-code handling, FormData) instead of the axios
 * instance in `./client`.
 *
 * The credential is the very same bearer token the app stores after a
 * successful login — see `stores/auth.ts` (which writes it) and the
 * request interceptor in `./client` (which reads it). Nothing here
 * changes the auth semantics of the backend: a missing or invalid
 * token still answers 401/403, the caller merely gets to decide what
 * to show for that answer.
 */

/**
 * localStorage key that holds the session bearer token.
 * Canonical writers/readers: `stores/auth.ts` and `api/client.ts`.
 */
export const AUTH_TOKEN_STORAGE_KEY = 'auth_token'

/** The bearer token of the current session, if the user is signed in. */
export function getAuthToken(): string | null {
  return localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)
}

/**
 * `fetch` with the session's bearer token attached.
 *
 * Unlike the axios client — which reacts to a 401 by clearing the
 * token and redirecting to `/login` — the caller keeps full control of
 * the response, so a 401 can be reported as an auth problem instead of
 * being mistaken for an unreachable backend.
 */
export async function authFetch(input: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  const token = getAuthToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  return fetch(input, { ...init, headers })
}
