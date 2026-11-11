<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import {
  PlusIcon,
  KeyIcon,
  PencilIcon,
  TrashIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  CheckCircleIcon,
  XCircleIcon,
  EyeIcon,
  EyeSlashIcon,
} from '@heroicons/vue/24/outline'
import { Dialog, DialogPanel, DialogTitle, TransitionRoot, TransitionChild } from '@headlessui/vue'
import { useCredentialsStore } from '@/stores'
import type {
  Credential,
  CredentialSchema,
  CredentialSchemaProperty,
  CredentialTestResult,
} from '@/types/api'

const route = useRoute()

const credentialsStore = useCredentialsStore()

// Form state
const showModal = ref(false)
const editingCredential = ref<Credential | null>(null)
const form = ref({
  name: '',
  type: '',
  data: {} as Record<string, unknown>,
})

const currentSchema = ref<CredentialSchema | null>(null)
const schemaLoading = ref(false)
const testResult = ref<CredentialTestResult | null>(null)
const testing = ref(false)
const loadingCredential = ref(false)
const lastSchemaType = ref('')

// ---------------------------------------------------------------------------
// Draft persistence
//
// A browser may discard and reload a background tab (Chrome tab
// discarding). Without a draft the reloaded page comes back with an
// empty form and the scroll position at the top — the user's input is
// gone. The draft below is written to sessionStorage on every change,
// so a reload restores the open modal, its values and its scroll
// position.
// ---------------------------------------------------------------------------
const DRAFT_KEY = 'm9m:credential-edit-draft'

interface CredentialDraft {
  id: string | null
  name: string
  type: string
  data: Record<string, unknown>
  scrollTop: number
  savedAt: number
}

const panelRef = ref<HTMLElement | null>(null)
const panelScrollTop = ref(0)

// DialogPanel is a component, so the template ref is the component
// instance — unwrap it to the rendered element.
const setPanelRef = (el: unknown) => {
  if (!el) {
    panelRef.value = null
    return
  }
  const instance = el as { $el?: HTMLElement }
  panelRef.value =
    instance.$el instanceof HTMLElement ? instance.$el : (el as HTMLElement)
}

const readDraft = (): CredentialDraft | null => {
  try {
    const raw = sessionStorage.getItem(DRAFT_KEY)
    if (!raw) return null
    const draft = JSON.parse(raw) as CredentialDraft
    if (!draft || typeof draft !== 'object' || !draft.type) return null
    return draft
  } catch {
    return null
  }
}

const writeDraft = () => {
  if (!showModal.value) return
  try {
    sessionStorage.setItem(
      DRAFT_KEY,
      JSON.stringify({
        id: editingCredential.value?.id ?? null,
        name: form.value.name,
        type: form.value.type,
        data: form.value.data,
        scrollTop: panelScrollTop.value,
        savedAt: Date.now(),
      } satisfies CredentialDraft)
    )
  } catch {
    // sessionStorage unavailable (private mode / quota) — not fatal.
  }
}

const clearDraft = () => {
  try {
    sessionStorage.removeItem(DRAFT_KEY)
  } catch {
    // ignore
  }
}

let draftTimer: ReturnType<typeof setTimeout> | null = null
const scheduleDraftSave = () => {
  if (draftTimer) clearTimeout(draftTimer)
  draftTimer = setTimeout(writeDraft, 250)
}

const onPanelScroll = (event: Event) => {
  panelScrollTop.value = (event.target as HTMLElement).scrollTop
  scheduleDraftSave()
}

const restorePanelScroll = (scrollTop: number) => {
  nextTick(() => {
    const el = panelRef.value
    if (!el) return
    el.scrollTop = scrollTop
    panelScrollTop.value = scrollTop
  })
}

// Keep the draft in sync with every keystroke.
watch(
  form,
  () => {
    if (!showModal.value) return
    scheduleDraftSave()
  },
  { deep: true }
)

// If a re-render (test result, schema load, …) resets the panel scroll
// to the top while the content is still scrollable, put the user back
// where they were. A deliberate scroll to the top is not undone because
// `panelScrollTop` is updated by every scroll event.
watch([testResult, loadingCredential, currentSchema, schemaLoading], async () => {
  await nextTick()
  const el = panelRef.value
  if (!el) return
  if (el.scrollTop === 0 && panelScrollTop.value > 0 && el.scrollHeight > el.clientHeight) {
    el.scrollTop = panelScrollTop.value
  }
})

// Grouping & expand state
const expandedGroups = ref<Set<string>>(new Set())
const toggleGroup = (type: string) => {
  if (expandedGroups.value.has(type)) {
    expandedGroups.value.delete(type)
  } else {
    expandedGroups.value.add(type)
  }
}

// Sorted group order (alphabetical by type)
const groupedCredentials = computed(() => {
  const out: Array<{ type: string; items: Credential[] }> = []
  const groups = credentialsStore.credentialsByType
  Object.keys(groups)
    .sort()
    .forEach((type) => {
      out.push({ type, items: groups[type] })
    })
  return out
})

// Open / close modal lifecycle
onMounted(async () => {
  await credentialsStore.fetchCredentials()
  await credentialsStore.fetchCredentialTypes()
  // Expand all groups by default — n8n shows them inline.
  Object.keys(credentialsStore.credentialsByType).forEach((t) =>
    expandedGroups.value.add(t)
  )

  // If we navigated here from NodePanel's "Create new credential" link,
  // auto-open the modal pre-filled with the requested type.
  const newType = route.query.newType
  if (typeof newType === 'string' && newType.length > 0) {
    openCreateModal(newType)
    return
  }

  // Restore an unsaved draft (the browser reloaded the tab — Chrome
  // discards background tabs — or the user navigated back). Without this
  // the form would come back empty and the scroll position lost.
  const draft = readDraft()
  if (!draft) return
  if (draft.id) {
    const credential = credentialsStore.credentials.find((c) => c.id === draft.id)
    if (credential) {
      await openEditModal(credential, draft)
      return
    }
  }
  openCreateModal(draft.type, draft)
  restorePanelScroll(draft.scrollTop)
})

const openCreateModal = (presetType = '', draft?: CredentialDraft) => {
  editingCredential.value = null
  form.value = {
    name: draft?.name ?? '',
    type: presetType || draft?.type || '',
    data: draft?.data ?? {},
  }
  testResult.value = null
  revealedFields.value = new Set()
  panelScrollTop.value = 0
  showModal.value = true
  writeDraft()
}

// Editing a credential loads its stored `data` so every field shows the
// real value and can be edited in place. The single-credential endpoint
// returns the payload (n8n editor parity); the list endpoint does not.
//
// `restore` carries a previously saved draft (page reload / tab discard)
// so the user's unsaved edits are not thrown away.
const openEditModal = async (credential: Credential, restore?: CredentialDraft) => {
  editingCredential.value = credential
  form.value = {
    name: credential.name,
    type: credential.type,
    data: {},
  }
  testResult.value = null
  revealedFields.value = new Set()
  panelScrollTop.value = 0
  showModal.value = true
  loadingCredential.value = true
  try {
    const full = await credentialsStore.fetchCredential(credential.id)
    form.value = {
      name: full.name,
      type: full.type,
      data: restore?.data
        ? { ...applyFieldAliases(full.data ?? {}), ...restore.data }
        : applyFieldAliases(full.data ?? {}),
    }
    if (restore?.scrollTop) restorePanelScroll(restore.scrollTop)
  } catch (e) {
    console.error('Failed to load credential data:', e)
    if (restore) {
      form.value = {
        name: restore.name,
        type: restore.type,
        data: restore.data,
      }
    }
  } finally {
    loadingCredential.value = false
  }
  writeDraft()
}

const closeModal = () => {
  showModal.value = false
  editingCredential.value = null
  form.value = { name: '', type: '', data: {} }
  currentSchema.value = null
  testResult.value = null
  revealedFields.value = new Set()
  panelScrollTop.value = 0
  clearDraft()
}

// Re-fetch schema whenever the type changes
watch(
  () => form.value.type,
  async (newType) => {
    if (!newType) return
    const unchanged = newType === lastSchemaType.value
    lastSchemaType.value = newType
    // Same type re-assigned (e.g. the edit modal re-rendering the
    // form with the loaded data) — keep the schema we already have.
    if (unchanged && currentSchema.value) return
    if (!unchanged) currentSchema.value = null
    schemaLoading.value = true
    try {
      const schema = await credentialsStore.fetchSchema(newType)
      currentSchema.value = schema
      // Pre-fill defaults only for NEW credentials. An existing
      // credential keeps its stored values — defaults must never
      // overwrite what the user saved.
      if (schema && !editingCredential.value) {
        for (const prop of schema.properties) {
          if (
            !(prop.name in form.value.data) &&
            prop.default !== undefined
          ) {
            form.value.data[prop.name] = prop.default
          }
        }
      }
    } finally {
      schemaLoading.value = false
    }
  }
)

// Generic field update
const updateData = (key: string, value: unknown) => {
  form.value.data = { ...form.value.data, [key]: value }
}

// Secret-looking fields are masked when the form has no schema for the
// credential type (fallback rendering).
const isSecretField = (key: string): boolean =>
  /pass(word|wd)?|secret|token|api[_-]?key|connectionstring|credential|private/i.test(
    key
  )

// Password fields can be revealed (eye toggle). Keyed by field name.
const revealedFields = ref<Set<string>>(new Set())
const isRevealed = (name: string): boolean => revealedFields.value.has(name)
const toggleReveal = (name: string) => {
  const next = new Set(revealedFields.value)
  if (next.has(name)) {
    next.delete(name)
  } else {
    next.add(name)
  }
  revealedFields.value = next
}

// Stored fields the schema does not describe (e.g. rabbitmq's
// `hostname` / `username`). They must stay visible and editable, not
// silently dropped from the form.
const extraDataFields = computed(() => {
  const schemaNames = new Set(
    (currentSchema.value?.properties ?? []).map((p) => p.name)
  )
  return Object.keys(form.value.data).filter((k) => !schemaNames.has(k))
})

// Synced n8n credentials sometimes use a different key for the same
// field (rabbitmq stores `hostname` / `username` where the schema says
// `host` / `user`). Copy the value over so the schema field is not
// blank and the required-field check does not report a false alarm.
// The original key is kept — nothing is lost on save.
const fieldAliases: Record<string, string[]> = {
  host: ['hostname', 'server', 'serverName'],
  user: ['username', 'login', 'loginName'],
  database: ['db', 'dbName', 'databaseName'],
  password: ['pass', 'pwd'],
}

const applyFieldAliases = (
  data: Record<string, unknown>
): Record<string, unknown> => {
  const out = { ...data }
  for (const [canonical, aliases] of Object.entries(fieldAliases)) {
    if (out[canonical] !== undefined) continue
    for (const alias of aliases) {
      if (out[alias] !== undefined) {
        out[canonical] = out[alias]
        break
      }
    }
  }
  return out
}

const updateNumberField = (key: string, raw: string) => {
  const n = parseInt(raw, 10)
  form.value.data = { ...form.value.data, [key]: Number.isNaN(n) ? 0 : n }
}

const updateJsonField = (key: string, raw: string) => {
  try {
    const parsed = raw.trim() === '' ? {} : JSON.parse(raw)
    form.value.data = { ...form.value.data, [key]: parsed }
  } catch {
    // Leave raw — show inline error on save.
  }
}

// Save / Test / Delete
const saveCredential = async () => {
  try {
    if (editingCredential.value) {
      await credentialsStore.updateCredential(
        editingCredential.value.id,
        form.value
      )
    } else {
      await credentialsStore.createCredential(form.value as any)
    }
    clearDraft()
    closeModal()
  } catch (e) {
    console.error('Failed to save credential:', e)
    alert(
      'Failed to save credential: ' +
        (e instanceof Error ? e.message : 'unknown error')
    )
  }
}

// Tests the values currently in the form — saved or not. The backend
// validates the payload it receives, so the result always reflects the
// data the user typed.
const runTest = async () => {
  if (!form.value.type) {
    alert('Select a credential type first.')
    return
  }
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await credentialsStore.testPayload(
      form.value.type,
      form.value.data
    )
  } catch (e) {
    testResult.value = {
      status: 'Error',
      message: e instanceof Error ? e.message : 'Test request failed',
    }
  } finally {
    testing.value = false
  }
}

const deleteCredential = async (credential: Credential) => {
  if (
    confirm(`Are you sure you want to delete the credential "${credential.name}"?`)
  ) {
    try {
      await credentialsStore.deleteCredential(credential.id)
    } catch (e) {
      console.error('Failed to delete credential:', e)
      alert('Failed to delete credential. Please try again.')
    }
  }
}

const formatDate = (dateStr: string) =>
  new Date(dateStr).toLocaleDateString()

// Schema-driven field renderer.
const fieldValue = (p: CredentialSchemaProperty): unknown =>
  form.value.data[p.name] ?? p.default ?? ''

const requiredFieldsMissing = computed(() => {
  if (!currentSchema.value) return []
  return currentSchema.value.required.filter((r) => {
    const v = form.value.data[r]
    return v === undefined || v === null || v === '' ||
      (typeof v === 'string' && v.trim() === '')
  })
})

// Icon for a credential type. Defaults to KeyIcon for unknown types.
const typeIcon = (_type: string) => KeyIcon

const typeLabel = (type: string): string => {
  const known: Record<string, string> = {
    httpBasicAuth: 'Basic Auth',
    httpHeaderAuth: 'Header Auth',
    oAuth2Api: 'OAuth2 API',
    apiKey: 'API Key',
    postgres: 'Postgres',
    mysql: 'MySQL',
    mongoDb: 'MongoDB',
    kafka: 'Kafka',
    slack: 'Slack',
    discord: 'Discord',
    openai: 'OpenAI',
    anthropic: 'Anthropic',
    jwtAuth: 'JWT Auth',
  }
  if (known[type]) return known[type]
  const fromBackend = credentialsStore.supportedCredentialTypes.find(
    (t) => t.type === type
  )
  return fromBackend?.displayName ?? type
}

// Credential-type chooser list — driven by the backend so it can never
// drift (mongoDb and kafka were missing from the old hardcoded list).
const fallbackCredentialTypes = [
  'httpBasicAuth',
  'httpHeaderAuth',
  'oAuth2Api',
  'apiKey',
  'postgres',
  'mysql',
  'mongoDb',
  'kafka',
  'slack',
  'discord',
  'openai',
  'anthropic',
  'jwtAuth',
]

const allCredentialTypes = computed(() => {
  const fromBackend = credentialsStore.supportedCredentialTypes
  if (fromBackend.length > 0) {
    return fromBackend.map((t) => t.type)
  }
  return fallbackCredentialTypes
})
</script>

<template>
  <div class="p-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">
          Credentials
        </h1>
        <p class="text-slate-500 dark:text-slate-400">
          Manage your authentication credentials securely
        </p>
      </div>
      <button
        @click="openCreateModal()"
        class="btn-primary flex items-center gap-2"
      >
        <PlusIcon class="w-5 h-5" />
        <span>Add Credential</span>
      </button>
    </div>

    <!-- Loading state -->
    <div v-if="credentialsStore.loading && credentialsStore.credentials.length === 0" class="text-center py-16">
      <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto" />
    </div>

    <!-- Empty state -->
    <div
      v-else-if="credentialsStore.credentials.length === 0"
      class="text-center py-16"
    >
      <KeyIcon class="w-16 h-16 mx-auto text-slate-300 dark:text-slate-600" />
      <h3 class="mt-4 text-lg font-medium text-slate-900 dark:text-white">
        No credentials yet
      </h3>
      <p class="mt-2 text-slate-500 dark:text-slate-400">
        Add credentials to use in your workflows
      </p>
      <button @click="openCreateModal()" class="mt-6 btn-primary">
        Add Credential
      </button>
    </div>

    <!-- Grouped credentials -->
    <div v-else class="space-y-3">
      <div
        v-for="group in groupedCredentials"
        :key="group.type"
        class="card overflow-hidden"
      >
        <!-- Group header -->
        <button
          @click="toggleGroup(group.type)"
          class="w-full flex items-center justify-between px-4 py-3 hover:bg-slate-50 dark:hover:bg-slate-700/40 transition-colors"
        >
          <div class="flex items-center gap-2">
            <ChevronDownIcon
              v-if="expandedGroups.has(group.type)"
              class="w-4 h-4 text-slate-400"
            />
            <ChevronRightIcon v-else class="w-4 h-4 text-slate-400" />
            <component
              :is="typeIcon(group.type)"
              class="w-5 h-5 text-primary-600 dark:text-primary-400"
            />
            <span class="font-semibold text-slate-900 dark:text-white">
              {{ typeLabel(group.type) }}
            </span>
            <span
              class="text-xs px-2 py-0.5 rounded-full bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300"
            >
              {{ group.items.length }}
            </span>
          </div>
        </button>

        <!-- Group rows -->
        <div v-if="expandedGroups.has(group.type)" class="border-t border-slate-200 dark:border-slate-700">
          <div
            v-for="cred in group.items"
            :key="cred.id"
            class="flex items-center justify-between px-4 py-3 hover:bg-slate-50 dark:hover:bg-slate-700/30 transition-colors"
          >
            <div class="flex-1 min-w-0">
              <h4 class="font-medium text-slate-900 dark:text-white truncate">
                {{ cred.name }}
              </h4>
              <p class="text-xs text-slate-500 dark:text-slate-400">
                Updated {{ formatDate(cred.updatedAt) }}
              </p>
            </div>
            <div class="flex items-center gap-1">
              <button
                @click="openEditModal(cred)"
                title="Edit credential"
                class="p-1.5 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700"
              >
                <PencilIcon class="w-4 h-4" />
              </button>
              <button
                @click="deleteCredential(cred)"
                title="Delete credential"
                class="p-1.5 rounded-lg text-slate-400 hover:text-red-600 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
              >
                <TrashIcon class="w-4 h-4" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Create / Edit modal -->
    <TransitionRoot :show="showModal" as="template">
      <Dialog @close="closeModal" class="relative z-50">
        <TransitionChild
          enter="ease-out duration-300"
          enter-from="opacity-0"
          enter-to="opacity-100"
          leave="ease-in duration-200"
          leave-from="opacity-100"
          leave-to="opacity-0"
        >
          <div class="fixed inset-0 bg-black/30 dark:bg-black/50" />
        </TransitionChild>

        <div class="fixed inset-0 flex items-center justify-center p-4">
          <TransitionChild
            enter="ease-out duration-300"
            enter-from="opacity-0 scale-95"
            enter-to="opacity-100 scale-100"
            leave="ease-in duration-200"
            leave-from="opacity-100 scale-100"
            leave-to="opacity-0 scale-95"
          >
            <DialogPanel
              :ref="setPanelRef"
              @scroll="onPanelScroll"
              class="w-full max-w-xl max-h-[90vh] overflow-y-auto overscroll-contain bg-white dark:bg-slate-800 rounded-xl shadow-xl"
            >
              <div class="p-6">
                <DialogTitle class="text-lg font-semibold text-slate-900 dark:text-white">
                  {{ editingCredential ? 'Edit Credential' : 'New Credential' }}
                </DialogTitle>

                <form @submit.prevent="saveCredential" class="mt-4 space-y-4">
                  <!-- Credential name (internal label) -->
                  <div>
                    <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                      Credential name
                    </label>
                    <input
                      v-model="form.name"
                      type="text"
                      class="input"
                      placeholder="My Postgres prod"
                      required
                    />
                  </div>

                  <!-- Type chooser -->
                  <div v-if="!editingCredential">
                    <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                      Type
                    </label>
                    <select v-model="form.type" class="input" required>
                      <option value="" disabled>Select a credential type...</option>
                      <option
                        v-for="t in allCredentialTypes"
                        :key="t"
                        :value="t"
                      >
                        {{ typeLabel(t) }} ({{ t }})
                      </option>
                    </select>
                  </div>
                  <div v-else>
                    <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                      Type
                    </label>
                    <div class="input bg-slate-50 dark:bg-slate-700/40 cursor-not-allowed">
                      {{ typeLabel(form.type) }}
                      <span class="text-xs text-slate-400 ml-2">({{ form.type }})</span>
                    </div>
                    <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">
                      Credential type cannot be changed after creation. Create a new credential to switch types.
                    </p>
                  </div>

                  <!-- Schema-driven fields -->
                  <div v-if="currentSchema && form.type" class="space-y-4 border-t border-slate-200 dark:border-slate-700 pt-4">
                    <p
                      v-if="loadingCredential"
                      class="text-xs text-slate-500 dark:text-slate-400"
                    >
                      Loading stored values…
                    </p>
                    <p
                      v-else-if="editingCredential"
                      class="text-xs text-sky-700 dark:text-sky-300 bg-sky-50 dark:bg-sky-900/20 px-3 py-2 rounded-lg"
                    >
                      Stored values are loaded below — edit any field directly and
                      save. Leave a password field untouched to keep it as-is.
                    </p>

                    <div
                      v-for="prop in currentSchema.properties"
                      :key="prop.name"
                    >
                      <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                        {{ prop.displayName }}
                        <span v-if="currentSchema.required.includes(prop.name)" class="text-red-500">*</span>
                      </label>

                      <!-- String input -->
                      <input
                        v-if="prop.type === 'string'"
                        type="text"
                        :value="fieldValue(prop)"
                        @input="updateData(prop.name, ($event.target as HTMLInputElement).value)"
                        :placeholder="prop.placeholder ?? ''"
                        class="input"
                      />

                      <!-- Password input (with reveal toggle) -->
                      <div v-else-if="prop.type === 'password'" class="relative">
                        <input
                          :type="isRevealed(prop.name) ? 'text' : 'password'"
                          autocomplete="off"
                          :value="fieldValue(prop)"
                          @input="updateData(prop.name, ($event.target as HTMLInputElement).value)"
                          :placeholder="prop.placeholder ?? ''"
                          class="input pr-10"
                        />
                        <button
                          type="button"
                          @click="toggleReveal(prop.name)"
                          :title="isRevealed(prop.name) ? 'Hide value' : 'Reveal value'"
                          class="absolute inset-y-0 right-0 flex items-center px-3 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
                        >
                          <EyeSlashIcon
                            v-if="isRevealed(prop.name)"
                            class="w-4 h-4"
                          />
                          <EyeIcon v-else class="w-4 h-4" />
                        </button>
                      </div>

                      <!-- Number input -->
                      <input
                        v-else-if="prop.type === 'number'"
                        type="number"
                        :value="fieldValue(prop)"
                        @input="updateNumberField(prop.name, ($event.target as HTMLInputElement).value)"
                        class="input"
                      />

                      <!-- Boolean toggle -->
                      <label
                        v-else-if="prop.type === 'boolean'"
                        class="flex items-center gap-2"
                      >
                        <input
                          type="checkbox"
                          :checked="!!fieldValue(prop)"
                          @change="updateData(prop.name, ($event.target as HTMLInputElement).checked)"
                          class="w-4 h-4 rounded border-slate-300 dark:border-slate-600 text-primary-600 focus:ring-primary-500"
                        />
                        <span class="text-sm text-slate-600 dark:text-slate-300">
                          {{ prop.description || prop.displayName }}
                        </span>
                      </label>

                      <!-- Options select -->
                      <select
                        v-else-if="prop.type === 'options' && prop.options"
                        :value="fieldValue(prop)"
                        @change="updateData(prop.name, ($event.target as HTMLSelectElement).value)"
                        class="input"
                      >
                        <option
                          v-for="opt in prop.options"
                          :key="String(opt.value)"
                          :value="opt.value"
                        >
                          {{ opt.name }}
                        </option>
                      </select>

                      <!-- JSON textarea -->
                      <textarea
                        v-else-if="prop.type === 'json'"
                        :value="JSON.stringify(fieldValue(prop) ?? {}, null, 2)"
                        @input="updateJsonField(prop.name, ($event.target as HTMLTextAreaElement).value)"
                        rows="4"
                        class="input font-mono text-sm"
                      />

                      <p v-if="prop.description" class="mt-1 text-xs text-slate-500 dark:text-slate-400">
                        {{ prop.description }}
                      </p>
                    </div>

                    <!-- Stored fields the schema does not describe
                         (e.g. rabbitmq's hostname / username). Keep them
                         visible and editable. -->
                    <div
                      v-if="extraDataFields.length > 0"
                      class="pt-3 border-t border-slate-100 dark:border-slate-700/60 space-y-4"
                    >
                      <p class="text-xs text-slate-500 dark:text-slate-400">
                        Additional stored fields
                      </p>
                      <div v-for="key in extraDataFields" :key="key">
                        <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                          {{ key }}
                        </label>
                        <div v-if="isSecretField(key)" class="relative">
                          <input
                            :type="isRevealed(key) ? 'text' : 'password'"
                            autocomplete="off"
                            :value="String(form.data[key] ?? '')"
                            @input="updateData(key, ($event.target as HTMLInputElement).value)"
                            class="input pr-10"
                          />
                          <button
                            type="button"
                            @click="toggleReveal(key)"
                            :title="isRevealed(key) ? 'Hide value' : 'Reveal value'"
                            class="absolute inset-y-0 right-0 flex items-center px-3 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
                          >
                            <EyeSlashIcon
                              v-if="isRevealed(key)"
                              class="w-4 h-4"
                            />
                            <EyeIcon v-else class="w-4 h-4" />
                          </button>
                        </div>
                        <input
                          v-else
                          type="text"
                          :value="String(form.data[key] ?? '')"
                          @input="updateData(key, ($event.target as HTMLInputElement).value)"
                          class="input"
                        />
                      </div>
                    </div>
                  </div>

                  <!-- Fallback for credential types without a form schema:
                       list the stored fields so they stay editable. -->
                  <div
                    v-else-if="!schemaLoading && form.type && Object.keys(form.data).length > 0"
                    class="space-y-4 border-t border-slate-200 dark:border-slate-700 pt-4"
                  >
                    <p class="text-xs text-slate-500 dark:text-slate-400">
                      This credential type has no form schema — the stored fields
                      are listed below and can be edited directly.
                    </p>
                    <div v-for="(value, key) in form.data" :key="String(key)">
                      <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                        {{ key }}
                      </label>
                      <div v-if="isSecretField(String(key))" class="relative">
                        <input
                          :type="isRevealed(String(key)) ? 'text' : 'password'"
                          autocomplete="off"
                          :value="value"
                          @input="updateData(String(key), ($event.target as HTMLInputElement).value)"
                          class="input pr-10"
                        />
                        <button
                          type="button"
                          @click="toggleReveal(String(key))"
                          :title="isRevealed(String(key)) ? 'Hide value' : 'Reveal value'"
                          class="absolute inset-y-0 right-0 flex items-center px-3 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
                        >
                          <EyeSlashIcon
                            v-if="isRevealed(String(key))"
                            class="w-4 h-4"
                          />
                          <EyeIcon v-else class="w-4 h-4" />
                        </button>
                      </div>
                      <input
                        v-else
                        type="text"
                        :value="value"
                        @input="updateData(String(key), ($event.target as HTMLInputElement).value)"
                        class="input"
                      />
                    </div>
                  </div>

                  <!-- Inline test result -->
                  <div v-if="testResult" class="border-t border-slate-200 dark:border-slate-700 pt-4">
                    <div
                      :class="[
                        'flex items-start gap-2 px-3 py-2 rounded-lg text-sm',
                        testResult.status === 'OK'
                          ? 'bg-green-50 dark:bg-green-900/20 text-green-700 dark:text-green-300'
                          : 'bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300'
                      ]"
                    >
                      <CheckCircleIcon v-if="testResult.status === 'OK'" class="w-5 h-5 flex-shrink-0 mt-0.5" />
                      <XCircleIcon v-else class="w-5 h-5 flex-shrink-0 mt-0.5" />
                      <div>
                        <div class="font-semibold">{{ testResult.status }}</div>
                        <div>{{ testResult.message }}</div>
                      </div>
                    </div>
                  </div>

                  <!-- Missing required warning -->
                  <p
                    v-if="requiredFieldsMissing.length > 0"
                    class="text-xs text-amber-700 dark:text-amber-300"
                  >
                    Missing required fields: {{ requiredFieldsMissing.join(', ') }}
                  </p>

                  <!-- Action row -->
                  <div class="flex justify-between items-center pt-4 border-t border-slate-200 dark:border-slate-700">
                    <button
                      type="button"
                      @click="runTest"
                      :disabled="testing || !form.type"
                      class="btn-secondary flex items-center gap-2"
                    >
                      <span v-if="!testing">Test connection</span>
                      <span v-else>Testing…</span>
                    </button>

                    <div class="flex gap-2">
                      <button
                        type="button"
                        @click="closeModal"
                        class="btn-secondary"
                      >
                        Cancel
                      </button>
                      <button type="submit" class="btn-primary">
                        {{ editingCredential ? 'Save' : 'Create' }}
                      </button>
                    </div>
                  </div>
                </form>
              </div>
            </DialogPanel>
          </TransitionChild>
        </div>
      </Dialog>
    </TransitionRoot>
  </div>
</template>
