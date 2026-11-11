import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { NodeCredentialType, NodeType } from '@/types/node'
import { getNodeTypes, getNodeType, getNodeTypeOptions } from '@/api/nodeTypes'

/**
 * Node-type catalog store.
 *
 * The catalog is the single source of truth for everything the parameter
 * panel renders: the property list, the `displayOptions` rules, the
 * `typeOptions` hints and the credential slots each node declares.
 *
 * The list endpoint (`GET /api/v1/node-types`) only embeds `properties`
 * for node types that declare them; the per-node endpoint
 * (`GET /api/v1/node-types/{name}`) always embeds the full property list.
 * The store therefore keeps a list cache plus a per-node detail cache.
 */
export const useNodeTypesStore = defineStore('nodeTypes', () => {
  // State
  const nodeTypes = ref<NodeType[]>([])
  const details = ref<Record<string, NodeType>>({})
  const loading = ref(false)
  const error = ref<string | null>(null)
  const loaded = ref(false)

  // Getters
  const nodeTypeMap = computed(() => {
    const map = new Map<string, NodeType>()
    nodeTypes.value.forEach((nt) => map.set(nt.name, nt))
    Object.values(details.value).forEach((nt) => map.set(nt.name, nt))
    return map
  })

  /** Look up a node type by its canonical name. */
  function byName(name: string): NodeType | undefined {
    return nodeTypeMap.value.get(name)
  }

  /**
   * Credential slots a node type declares. Metadata is authoritative;
   * an empty array means the node declares no credentials.
   */
  function credentialsFor(name: string): NodeCredentialType[] {
    return byName(name)?.credentials ?? []
  }

  /**
   * Node types grouped by their n8n `group` when the backend reports one,
   * otherwise by their `category`. Groups are the labels n8n shows in the
   * node palette ("Transform", "Core", …).
   */
  function groupedByCategory(): Record<string, NodeType[]> {
    const grouped: Record<string, NodeType[]> = {}
    nodeTypes.value.forEach((nt) => {
      const group = nt.group && nt.group.length > 0 ? nt.group[0] : nt.category
      const key = group && group.length > 0 ? group : 'Other'
      if (!grouped[key]) grouped[key] = []
      grouped[key].push(nt)
    })
    return grouped
  }

  // Actions
  async function fetchNodeTypes(force = false): Promise<NodeType[]> {
    if (loaded.value && !force) return nodeTypes.value
    loading.value = true
    error.value = null
    try {
      nodeTypes.value = await getNodeTypes()
      loaded.value = true
      return nodeTypes.value
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch node types'
      throw e
    } finally {
      loading.value = false
    }
  }

  /**
   * Fetch the full metadata (including `properties`) for a single node
   * type. The list endpoint omits `properties` for some nodes, so the
   * panel calls this when it needs the authoritative property list.
   */
  async function fetchNodeTypeDetail(name: string, force = false): Promise<NodeType | undefined> {
    if (details.value[name] && !force) return details.value[name]
    try {
      const detail = await getNodeType(name)
      details.value = { ...details.value, [name]: detail }
      return detail
    } catch (e) {
      error.value = e instanceof Error ? e.message : `Failed to fetch node type ${name}`
      return undefined
    }
  }

  /** Load the catalog once; safe to call from several components. */
  async function ensureLoaded(): Promise<NodeType[]> {
    if (loaded.value) return nodeTypes.value
    try {
      return await fetchNodeTypes()
    } catch {
      return nodeTypes.value
    }
  }

  /**
   * Probe the dynamic option endpoint for a node property. The backend
   * endpoint is optional — when it is missing the caller degrades to a
   * static/plain input instead of failing.
   */
  async function fetchOptions(
    name: string,
    property: string,
    params: Record<string, unknown> = {},
  ): Promise<{ name: string; value: string }[] | null> {
    try {
      return await getNodeTypeOptions(name, property, params)
    } catch {
      return null
    }
  }

  return {
    // State
    nodeTypes,
    details,
    loading,
    error,
    loaded,

    // Getters
    nodeTypeMap,
    byName,
    credentialsFor,
    groupedByCategory,

    // Actions
    fetchNodeTypes,
    fetchNodeTypeDetail,
    ensureLoaded,
    fetchOptions,
  }
})
