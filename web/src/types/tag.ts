/**
 * Tag wire types.
 *
 * The backend stores tags in their own table and keeps the tag IDs on
 * the workflow (`workflow.tags` is a list of tag IDs). The list
 * endpoint answers with an envelope that carries the same array twice
 * (`data` and `tags`); the API client normalises that to one array.
 *
 * Version types live in `./version` — they are shared with the
 * version-history feature and are NOT duplicated here.
 */

/** A single workflow tag. */
export interface Tag {
  id: string
  name: string
  /** Hex colour (`#RRGGBB`). May be empty when the tag was created without one. */
  color: string
  createdAt: string
  updatedAt: string
}

/** Body of `POST /api/v1/tags`. */
export interface TagCreateRequest {
  name: string
  color?: string
}

/** Body of `PUT|PATCH /api/v1/tags/{id}`. Omitted fields keep the stored value. */
export interface TagUpdateRequest {
  name?: string
  color?: string
}

/** Query parameters of `GET /api/v1/tags`. */
export interface TagListParams {
  search?: string
  limit?: number
  offset?: number
}

/**
 * Envelope of `GET /api/v1/tags`.
 *
 * `data` and `tags` hold the SAME array on the wire. The API client
 * always fills both fields from one canonical list so the rest of the
 * app can read either one.
 */
export interface TagListResponse {
  data: Tag[]
  tags: Tag[]
  total: number
  count: number
  limit: number
  offset: number
}

/**
 * Envelope of the workflow-tag endpoints
 * (`GET|POST /api/v1/workflows/{id}/tags`, `POST .../tags/{tagId}`).
 */
export interface WorkflowTagsResponse {
  /** Full tag objects resolved from the tag IDs stored on the workflow. */
  data: Tag[]
  count: number
  /** The resolved tag IDs (unknown IDs are dropped by the server). */
  tagIds: string[]
}

/**
 * Palette offered by the tag editor. Sane defaults — high-contrast
 * enough to stay readable on both light and dark surfaces.
 */
export const TAG_PALETTE: readonly string[] = [
  '#EF4444', // red
  '#F97316', // orange
  '#F59E0B', // amber
  '#10B981', // emerald
  '#14B8A6', // teal
  '#3B82F6', // blue
  '#6366F1', // indigo
  '#8B5CF6', // violet
  '#EC4899', // pink
  '#64748B', // slate
]

/** Colour used when a tag is created without an explicit one. */
export const DEFAULT_TAG_COLOR = '#6366F1'

/**
 * Normalises a user-supplied colour to a `#RRGGBB` string.
 * Returns `null` when the value is not a hex colour.
 */
export function normalizeHexColor(value: string | undefined | null): string | null {
  const raw = (value ?? '').trim()
  if (!raw) return null

  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(raw)
  if (short) {
    return `#${short[1]}${short[1]}${short[2]}${short[2]}${short[3]}${short[3]}`.toLowerCase()
  }

  const full = /^#([0-9a-f]{6})$/i.exec(raw)
  if (full) return raw.toLowerCase()

  return null
}
