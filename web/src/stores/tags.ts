import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { Tag, TagCreateRequest, TagListParams, TagUpdateRequest } from '@/types/tag'
import * as tagsApi from '@/api/tags'

/** Page size used when the store walks through the whole tag list. */
const TAG_PAGE_SIZE = 100

/** Safety net so a misbehaving server cannot spin the paging loop forever. */
const MAX_TAG_PAGES = 50

function dedupeById(tags: Tag[]): Tag[] {
  const seen = new Set<string>()
  const result: Tag[] = []
  for (const tag of tags) {
    if (!tag || seen.has(tag.id)) continue
    seen.add(tag.id)
    result.push(tag)
  }
  return result
}

export const useTagsStore = defineStore('tags', () => {
  // State
  const tags = ref<Tag[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const total = ref(0)

  /** Tags per workflow, keyed by workflow ID. */
  const workflowTags = ref<Record<string, Tag[]>>({})

  // Getters
  const count = computed(() => tags.value.length)

  const sortedTags = computed(() =>
    [...tags.value].sort((a, b) => a.name.localeCompare(b.name))
  )

  const tagMap = computed<Record<string, Tag>>(() => {
    const map: Record<string, Tag> = {}
    for (const tag of tags.value) {
      map[tag.id] = tag
    }
    return map
  })

  function getTag(id: string): Tag | undefined {
    return tagMap.value[id]
  }

  function getWorkflowTagList(workflowId: string): Tag[] {
    return workflowTags.value[workflowId] ?? []
  }

  // Actions

  /**
   * Loads one page of tags. Used for the server-side search box.
   */
  async function fetchTags(params?: TagListParams): Promise<Tag[]> {
    loading.value = true
    error.value = null
    try {
      const response = await tagsApi.listTags(params)
      tags.value = response.data
      total.value = response.total
      return tags.value
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to load tags')
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Loads the complete tag list. The endpoint is paginated, so the
   * store walks the pages until every tag is collected.
   */
  async function fetchAllTags(): Promise<Tag[]> {
    loading.value = true
    error.value = null
    try {
      const collected: Tag[] = []
      let offset = 0
      let reportedTotal = 0

      for (let page = 0; page < MAX_TAG_PAGES; page += 1) {
        const response = await tagsApi.listTags({
          limit: TAG_PAGE_SIZE,
          offset,
        })
        collected.push(...response.data)
        reportedTotal = response.total

        if (response.data.length === 0) break
        if (collected.length >= reportedTotal) break
        offset += response.data.length
      }

      tags.value = dedupeById(collected)
      total.value = reportedTotal
      return tags.value
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to load tags')
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Creates a tag and adds it to the store.
   */
  async function createTag(data: TagCreateRequest): Promise<Tag> {
    loading.value = true
    error.value = null
    try {
      const tag = await tagsApi.createTag(data)
      tags.value = [...tags.value, tag]
      total.value = tags.value.length
      return tag
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to create the tag')
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Renames and/or recolours a tag.
   */
  async function updateTag(id: string, data: TagUpdateRequest): Promise<Tag> {
    loading.value = true
    error.value = null
    try {
      const updated = await tagsApi.updateTag(id, data)
      const index = tags.value.findIndex((tag) => tag.id === id)
      if (index !== -1) {
        tags.value[index] = updated
      } else {
        tags.value = [...tags.value, updated]
      }
      // Keep the per-workflow caches in sync with the new name/colour.
      for (const workflowId of Object.keys(workflowTags.value)) {
        workflowTags.value[workflowId] = workflowTags.value[workflowId].map((tag) =>
          tag.id === id ? updated : tag
        )
      }
      return updated
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to update the tag')
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Deletes a tag. The backend refuses the delete when the tag is still
   * attached to a workflow, so the error message is forwarded verbatim.
   */
  async function deleteTag(id: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await tagsApi.deleteTag(id)
      tags.value = tags.value.filter((tag) => tag.id !== id)
      total.value = tags.value.length
      for (const workflowId of Object.keys(workflowTags.value)) {
        workflowTags.value[workflowId] = workflowTags.value[workflowId].filter(
          (tag) => tag.id !== id
        )
      }
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to delete the tag')
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Loads the tags of one workflow.
   */
  async function fetchWorkflowTags(workflowId: string): Promise<Tag[]> {
    try {
      const response = await tagsApi.getWorkflowTags(workflowId)
      workflowTags.value = { ...workflowTags.value, [workflowId]: response.data }
      return response.data
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to load the workflow tags')
      throw e
    }
  }

  /**
   * Replaces the tag list of a workflow.
   */
  async function setWorkflowTags(workflowId: string, tagIds: string[]): Promise<Tag[]> {
    loading.value = true
    error.value = null
    try {
      const response = await tagsApi.setWorkflowTags(workflowId, tagIds)
      workflowTags.value = { ...workflowTags.value, [workflowId]: response.data }
      return response.data
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to update the workflow tags')
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Attaches one tag to a workflow.
   */
  async function addWorkflowTag(workflowId: string, tagId: string): Promise<Tag[]> {
    loading.value = true
    error.value = null
    try {
      const response = await tagsApi.addWorkflowTag(workflowId, tagId)
      workflowTags.value = { ...workflowTags.value, [workflowId]: response.data }
      return response.data
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to attach the tag')
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Detaches one tag from a workflow.
   */
  async function removeWorkflowTag(workflowId: string, tagId: string): Promise<void> {
    loading.value = true
    error.value = null
    try {
      await tagsApi.removeWorkflowTag(workflowId, tagId)
      const current = workflowTags.value[workflowId] ?? []
      workflowTags.value = {
        ...workflowTags.value,
        [workflowId]: current.filter((tag) => tag.id !== tagId),
      }
    } catch (e) {
      error.value = tagsApi.tagErrorMessage(e, 'Failed to detach the tag')
      throw e
    } finally {
      loading.value = false
    }
  }

  function clearError() {
    error.value = null
  }

  return {
    // State
    tags,
    loading,
    error,
    total,
    workflowTags,

    // Getters
    count,
    sortedTags,
    tagMap,

    // Actions
    fetchTags,
    fetchAllTags,
    createTag,
    updateTag,
    deleteTag,
    fetchWorkflowTags,
    setWorkflowTags,
    addWorkflowTag,
    removeWorkflowTag,
    getTag,
    getWorkflowTagList,
    clearError,
  }
})
