import apiClient from './client'
import * as versionsApi from './versions'
import type {
  Tag,
  TagCreateRequest,
  TagListParams,
  TagListResponse,
  TagUpdateRequest,
  WorkflowTagsResponse,
} from '@/types/tag'
import type {
  VersionComparison,
  VersionCreateRequest,
  VersionListResponse,
  VersionRestoreRequest,
  WorkflowVersion,
} from '@/types/version'

/**
 * Client for the tag feature and the workflow-version feature.
 *
 * Envelope handling
 * -----------------
 * * `GET /tags` answers with `{data, tags, total, count, limit, offset}`
 *   where `data` and `tags` are the SAME array. `data` is the canonical
 *   field; the client normalises the payload so both fields always hold
 *   the same (possibly empty) list.
 * * The workflow-tag endpoints answer with `{data, count, tagIds}`.
 * * The version list answers with `{data, total, count, limit, offset}`.
 * * Single tag / single version objects are flat (no envelope).
 * * `DELETE` answers with `204 No Content` — there is no body to unwrap.
 */

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function toTagArray(value: unknown): Tag[] {
  return Array.isArray(value) ? (value as Tag[]) : []
}

/**
 * Normalises the `GET /tags` envelope. `data` wins; `tags` is only used
 * when the server did not send a usable `data` array.
 */
function normalizeTagList(body: unknown): TagListResponse {
  const payload = (body ?? {}) as Record<string, unknown>
  const data = toTagArray(payload.data)
  const alias = toTagArray(payload.tags)
  const list = data.length > 0 || alias.length === 0 ? data : alias

  const numberOf = (value: unknown, fallback: number): number =>
    typeof value === 'number' && Number.isFinite(value) ? value : fallback

  return {
    data: list,
    tags: list,
    total: numberOf(payload.total, list.length),
    count: numberOf(payload.count, list.length),
    limit: numberOf(payload.limit, list.length),
    offset: numberOf(payload.offset, 0),
  }
}

/** Normalises the `{data, count, tagIds}` envelope of the workflow-tag endpoints. */
function normalizeWorkflowTags(body: unknown): WorkflowTagsResponse {
  const payload = (body ?? {}) as Record<string, unknown>
  const data = toTagArray(payload.data)
  const tagIds = Array.isArray(payload.tagIds)
    ? payload.tagIds.map((id) => String(id))
    : data.map((tag) => tag.id)

  return {
    data,
    count: typeof payload.count === 'number' ? payload.count : data.length,
    tagIds,
  }
}

/**
 * Reads the human-readable message out of an API error.
 *
 * The m9m error envelope is `{"error":true,"message":"…","code":404}`;
 * axios wraps it in `error.response.data`.
 */
export function tagErrorMessage(error: unknown, fallback: string): string {
  const body = (error as { response?: { data?: unknown } })?.response?.data

  if (body && typeof body === 'object' && 'message' in body) {
    const message = (body as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
  }

  if (error instanceof Error && error.message) return error.message

  return fallback
}

// ---------------------------------------------------------------------------
// Tags
// ---------------------------------------------------------------------------

/**
 * Lists the tags.
 *
 * GET /api/v1/tags?search=&limit=&offset=
 * → `{data, tags, total, count, limit, offset}`
 */
export async function listTags(params?: TagListParams): Promise<TagListResponse> {
  const query: Record<string, string | number> = {}
  if (params?.search) query.search = params.search
  if (params?.limit !== undefined) query.limit = params.limit
  if (params?.offset !== undefined) query.offset = params.offset

  const response = await apiClient.get('/tags', { params: query })
  return normalizeTagList(response.data)
}

/**
 * Creates a tag.
 *
 * POST /api/v1/tags `{name, color}` → 201 with the flat tag object.
 */
export async function createTag(data: TagCreateRequest): Promise<Tag> {
  const response = await apiClient.post<Tag>('/tags', data)
  return response.data
}

/**
 * Reads one tag.
 *
 * GET /api/v1/tags/{id} → flat tag object.
 */
export async function getTag(id: string): Promise<Tag> {
  const response = await apiClient.get<Tag>(`/tags/${encodeURIComponent(id)}`)
  return response.data
}

/**
 * Renames and/or recolours a tag.
 *
 * PUT /api/v1/tags/{id} `{name?, color?}` → flat tag object.
 */
export async function updateTag(id: string, data: TagUpdateRequest): Promise<Tag> {
  const response = await apiClient.put<Tag>(`/tags/${encodeURIComponent(id)}`, data)
  return response.data
}

/**
 * Deletes a tag.
 *
 * DELETE /api/v1/tags/{id} → 204 No Content (no body to unwrap).
 */
export async function deleteTag(id: string): Promise<void> {
  await apiClient.delete(`/tags/${encodeURIComponent(id)}`)
}

// ---------------------------------------------------------------------------
// Workflow tags
// ---------------------------------------------------------------------------

/**
 * Reads the tags assigned to a workflow.
 *
 * GET /api/v1/workflows/{id}/tags → `{data, count, tagIds}`
 */
export async function getWorkflowTags(workflowId: string): Promise<WorkflowTagsResponse> {
  const response = await apiClient.get(
    `/workflows/${encodeURIComponent(workflowId)}/tags`
  )
  return normalizeWorkflowTags(response.data)
}

/**
 * Replaces the tag list of a workflow.
 *
 * POST /api/v1/workflows/{id}/tags `{tagIds}` → `{data, count, tagIds}`
 */
export async function setWorkflowTags(
  workflowId: string,
  tagIds: string[]
): Promise<WorkflowTagsResponse> {
  const response = await apiClient.post(
    `/workflows/${encodeURIComponent(workflowId)}/tags`,
    { tagIds }
  )
  return normalizeWorkflowTags(response.data)
}

/**
 * Attaches one tag to a workflow.
 *
 * POST /api/v1/workflows/{id}/tags/{tagId} → `{data, count, tagIds}`
 */
export async function addWorkflowTag(
  workflowId: string,
  tagId: string
): Promise<WorkflowTagsResponse> {
  const response = await apiClient.post(
    `/workflows/${encodeURIComponent(workflowId)}/tags/${encodeURIComponent(tagId)}`
  )
  return normalizeWorkflowTags(response.data)
}

/**
 * Detaches one tag from a workflow.
 *
 * DELETE /api/v1/workflows/{id}/tags/{tagId} → 204 No Content.
 */
export async function removeWorkflowTag(workflowId: string, tagId: string): Promise<void> {
  await apiClient.delete(
    `/workflows/${encodeURIComponent(workflowId)}/tags/${encodeURIComponent(tagId)}`
  )
}

// ---------------------------------------------------------------------------
// Workflow versions
//
// The canonical implementation lives in `./versions` (it predates this
// module and is used by the version store). It is re-exported here so
// the complete tag + version surface is available from one place, with
// the envelope normalised the same way as everywhere else.
// ---------------------------------------------------------------------------

/**
 * Lists the versions of a workflow.
 *
 * GET /api/v1/workflows/{id}/versions?limit=&offset=
 * → `{data, total, count, limit, offset}`
 */
export async function listVersions(
  workflowId: string,
  limit = 50,
  offset = 0
): Promise<VersionListResponse> {
  const response = await versionsApi.listVersions(workflowId, limit, offset)
  return {
    ...response,
    data: Array.isArray(response.data) ? response.data : [],
  }
}

/**
 * Reads one version.
 *
 * GET /api/v1/workflows/{id}/versions/{versionId} → flat version object.
 */
export async function getVersion(
  workflowId: string,
  versionId: string
): Promise<WorkflowVersion> {
  return versionsApi.getVersion(workflowId, versionId)
}

/**
 * Creates a version snapshot.
 *
 * POST /api/v1/workflows/{id}/versions → 201 with the flat version object.
 */
export async function createVersion(
  workflowId: string,
  data: VersionCreateRequest
): Promise<WorkflowVersion> {
  return versionsApi.createVersion(workflowId, data)
}

/**
 * Deletes a version snapshot.
 *
 * DELETE /api/v1/workflows/{id}/versions/{versionId} → 204 No Content.
 */
export async function deleteVersion(workflowId: string, versionId: string): Promise<void> {
  await versionsApi.deleteVersion(workflowId, versionId)
}

/**
 * Restores a workflow to a version.
 *
 * POST /api/v1/workflows/{id}/versions/{versionId}/restore
 * → `{message, restoredFrom, backupCreated}`
 */
export async function restoreVersion(
  workflowId: string,
  versionId: string,
  data?: VersionRestoreRequest
): Promise<{ message: string; restoredFrom: WorkflowVersion; backupCreated: boolean }> {
  return versionsApi.restoreVersion(workflowId, versionId, data)
}

/**
 * Compares two versions.
 *
 * GET /api/v1/workflows/{id}/versions/compare?from=&to=
 * → `{fromVersion, toVersion, changes}`
 */
export async function compareVersions(
  workflowId: string,
  fromVersionId: string,
  toVersionId: string
): Promise<VersionComparison> {
  return versionsApi.compareVersions(workflowId, fromVersionId, toVersionId)
}
