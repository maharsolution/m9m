import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type {
  Credential,
  CredentialCreate,
  CredentialSchema,
  CredentialTestResult,
} from '@/types/api'
import * as credentialsApi from '@/api/credentials'

export const useCredentialsStore = defineStore('credentials', () => {
  // State
  const credentials = ref<Credential[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  // Schema cache keyed by credential type. null = not yet fetched.
  // Missing key after fetch means the backend returned 404.
  const schemaCache = ref<Record<string, CredentialSchema | null>>({})

  // Getters
  const credentialsByType = computed(() => {
    const grouped: Record<string, Credential[]> = {}
    credentials.value.forEach((c) => {
      if (!grouped[c.type]) {
        grouped[c.type] = []
      }
      grouped[c.type].push(c)
    })
    return grouped
  })

  const credentialTypes = computed(() => {
    return [...new Set(credentials.value.map((c) => c.type))].sort()
  })

  const count = computed(() => credentials.value.length)

  // Actions
  async function fetchCredentials() {
    loading.value = true
    error.value = null
    try {
      credentials.value = await credentialsApi.getCredentials()
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to fetch credentials'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function createCredential(credential: CredentialCreate) {
    loading.value = true
    error.value = null
    try {
      const newCredential = await credentialsApi.createCredential(credential)
      credentials.value.push(newCredential)
      return newCredential
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to create credential'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function updateCredential(
    id: string,
    updates: Partial<CredentialCreate>
  ) {
    loading.value = true
    error.value = null
    try {
      const updated = await credentialsApi.updateCredential(id, updates)
      const index = credentials.value.findIndex((c) => c.id === id)
      if (index !== -1) {
        credentials.value[index] = updated
      }
      return updated
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to update credential'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function deleteCredential(id: string) {
    loading.value = true
    error.value = null
    try {
      await credentialsApi.deleteCredential(id)
      credentials.value = credentials.value.filter((c) => c.id !== id)
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to delete credential'
      throw e
    } finally {
      loading.value = false
    }
  }

  async function fetchSchema(type: string): Promise<CredentialSchema | null> {
    if (type in schemaCache.value) {
      return schemaCache.value[type]
    }
    try {
      const schema = await credentialsApi.getCredentialSchema(type)
      schemaCache.value[type] = schema
      return schema
    } catch (e) {
      // Unknown type: cache as null so we don't keep retrying.
      schemaCache.value[type] = null
      return null
    }
  }

  async function testConnection(id: string): Promise<CredentialTestResult> {
    return credentialsApi.testCredential(id)
  }

  function getCredential(id: string): Credential | undefined {
    return credentials.value.find((c) => c.id === id)
  }

  function getCredentialsByType(type: string): Credential[] {
    return credentials.value.filter((c) => c.type === type)
  }

  return {
    // State
    credentials,
    loading,
    error,
    schemaCache,

    // Getters
    credentialsByType,
    credentialTypes,
    count,

    // Actions
    fetchCredentials,
    createCredential,
    updateCredential,
    deleteCredential,
    fetchSchema,
    testConnection,
    getCredential,
    getCredentialsByType,
  }
})
