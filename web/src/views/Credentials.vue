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
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
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

// Shared control styling (token layer: neutral ramp, 8px radius).
const CONTROL_CLASS =
  'h-10 w-full rounded-lg border border-neutral-300 bg-white px-3 text-sm text-neutral-950 transition-colors duration-200 placeholder:text-neutral-400 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 disabled:opacity-50 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10'

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
  const type = presetType || draft?.type || ''
  form.value = {
    name: draft?.name ?? '',
    type,
    // The alias keys are folded into the canonical fields, so the form
    // shows one username / one password even when the draft was written
    // by an older build that used the `saslUsername` spelling.
    data: applyFieldAliases(draft?.data ?? {}, type),
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
      // The stored data and the restored draft are merged first, then
      // the alias keys are folded into the canonical fields — the draft
      // wins, and the form still shows a single username / password.
      data: applyFieldAliases(
        restore?.data
          ? { ...(full.data ?? {}), ...restore.data }
          : full.data ?? {},
        full.type
      ),
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

// Type specific aliases. The Kafka credential stores the login as
// `username` / `password` — that is the n8n shape (see
// n8n/packages/nodes-base/credentials/Kafka.credentials.ts) and the
// canonical m9m field name. Credentials written by an older m9m build
// still carry the `saslUsername` / `saslPassword` spellings, so those
// are folded into the canonical fields.
const typeFieldAliases: Record<string, Record<string, string[]>> = {
  kafka: {
    username: ['saslUsername', 'user'],
    password: ['saslPassword', 'pass', 'pwd'],
  },
}

// Generic aliases that must not be applied for a given type. The
// generic `user` ← `username` mapping would create a THIRD username
// input for Kafka (it would land in "Additional stored fields").
const typeAliasBlocklist: Record<string, string[]> = {
  kafka: ['user'],
}

// Alias keys that are folded into a canonical field. They must not be
// rendered again: they would duplicate the canonical input and a stale
// copy could shadow the value the operator just edited.
const typeAliasKeys = (type: string): Set<string> =>
  new Set(Object.values(typeFieldAliases[type] ?? {}).flat())

// Stored fields the schema does not describe (e.g. rabbitmq's
// `hostname` / `username`). They must stay visible and editable, not
// silently dropped from the form.
const extraDataFields = computed(() => {
  const schemaNames = new Set(
    (currentSchema.value?.properties ?? []).map((p) => p.name)
  )
  // Alias keys of a canonical field are not "additional" fields — they
  // would render a second input for the very same value (the Kafka
  // credential used to show the username three times).
  const aliasNames = typeAliasKeys(form.value.type)
  return Object.keys(form.value.data).filter(
    (k) => !schemaNames.has(k) && !aliasNames.has(k)
  )
})

const applyFieldAliases = (
  data: Record<string, unknown>,
  type = ''
): Record<string, unknown> => {
  const out = { ...data }
  const generic = { ...fieldAliases }
  for (const blocked of typeAliasBlocklist[type] ?? []) {
    delete generic[blocked]
  }
  const aliases = { ...generic, ...(typeFieldAliases[type] ?? {}) }
  for (const [canonical, aliasList] of Object.entries(aliases)) {
    if (out[canonical] !== undefined) continue
    for (const alias of aliasList) {
      if (out[alias] !== undefined) {
        out[canonical] = out[alias]
        break
      }
    }
  }
  // The alias keys are dropped once their value has been folded into
  // the canonical field: the form then shows exactly ONE username and ONE
  // password, and a stale copy can no longer shadow the edited value.
  for (const alias of typeAliasKeys(type)) {
    delete out[alias]
  }
  return out
}

// Kafka keeps a single username / password pair in the form. The legacy
// `saslUsername` / `saslPassword` keys are written with the same value so
// the node's credential bridge (`ApplyKafkaCredentialAuth`) and the
// credential connection test always use the login the operator sees — a
// stale copy left over from an earlier save would otherwise win at
// runtime.
const normalizeCredentialData = (
  type: string,
  data: Record<string, unknown>
): Record<string, unknown> => {
  if (type !== 'kafka') return data
  const out = { ...data }
  out.saslUsername = out.username ?? ''
  out.saslPassword = out.password ?? ''
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
    const payload = {
      name: form.value.name,
      type: form.value.type,
      data: normalizeCredentialData(form.value.type, form.value.data),
    }
    if (editingCredential.value) {
      await credentialsStore.updateCredential(
        editingCredential.value.id,
        payload
      )
    } else {
      await credentialsStore.createCredential(payload as any)
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
      normalizeCredentialData(form.value.type, form.value.data)
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

// Options list for a select field. When the stored value is not one of
// the schema options (e.g. kafka stores `authentication: true` where the
// schema expects none/basic/apiKey) the real value is added as an extra
// option so it stays visible instead of silently rendering blank.
const optionValues = (
  p: CredentialSchemaProperty
): Array<{ name: string; value: string }> => {
  const options = (p.options ?? []).map((o) => ({
    name: o.name,
    value: String(o.value),
  }))
  const current = fieldValue(p)
  if (
    current === '' ||
    current === undefined ||
    current === null ||
    options.some((o) => o.value === String(current))
  ) {
    return options
  }
  return [...options, { name: String(current), value: String(current) }]
}

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
  'rabbitmq',
  'redis',
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
  <div>
    <!-- Page header -->
    <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
          Credentials
        </h1>
        <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
          Manage your authentication credentials securely
        </p>
      </div>
      <Button
        variant="primary"
        size="sm"
        @click="openCreateModal()"
      >
        <template #icon>
          <PlusIcon class="h-4 w-4" />
        </template>
        <span>Add credential</span>
      </Button>
    </div>

    <!-- Loading state -->
    <div
      v-if="credentialsStore.loading && credentialsStore.credentials.length === 0"
      class="flex flex-col items-center justify-center rounded-lg border border-neutral-200 bg-white py-16 dark:border-neutral-800 dark:bg-neutral-900"
    >
      <span
        class="inline-block h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
        aria-hidden="true"
      />
    </div>

    <!-- Empty state -->
    <div
      v-else-if="credentialsStore.credentials.length === 0"
      class="flex flex-col items-center rounded-lg border border-neutral-200 bg-white px-4 py-16 text-center dark:border-neutral-800 dark:bg-neutral-900"
    >
      <KeyIcon class="h-12 w-12 text-neutral-300 dark:text-neutral-700" />
      <h3 class="mt-4 text-lg font-semibold text-neutral-950 dark:text-neutral-50">
        No credentials yet
      </h3>
      <p class="mt-1 text-sm text-neutral-600 dark:text-neutral-400">
        Add credentials to use in your workflows
      </p>
      <Button
        variant="primary"
        size="sm"
        class="mt-6"
        @click="openCreateModal()"
      >
        Add credential
      </Button>
    </div>

    <!-- Grouped credentials -->
    <div
      v-else
      class="space-y-3"
    >
      <div
        v-for="group in groupedCredentials"
        :key="group.type"
        class="overflow-hidden rounded-lg border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-900"
      >
        <!-- Group header -->
        <button
          type="button"
          class="flex w-full items-center justify-between px-4 py-3 text-left transition-colors duration-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/40"
          @click="toggleGroup(group.type)"
        >
          <span class="flex items-center gap-2">
            <ChevronDownIcon
              v-if="expandedGroups.has(group.type)"
              class="h-4 w-4 text-neutral-400"
            />
            <ChevronRightIcon
              v-else
              class="h-4 w-4 text-neutral-400"
            />
            <component
              :is="typeIcon(group.type)"
              class="h-4 w-4 text-brand-600 dark:text-brand-400"
            />
            <span class="text-sm font-semibold text-neutral-950 dark:text-neutral-50">
              {{ typeLabel(group.type) }}
            </span>
            <span class="inline-flex h-5 items-center rounded-lg border border-neutral-200 bg-neutral-100 px-2 text-2xs font-medium text-neutral-600 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-300">
              {{ group.items.length }}
            </span>
          </span>
        </button>

        <!-- Group rows. The secret values are never part of a row —
             only the name, the type and the configured state. -->
        <div
          v-if="expandedGroups.has(group.type)"
          class="border-t border-neutral-200 dark:border-neutral-800"
        >
          <div
            v-for="cred in group.items"
            :key="cred.id"
            class="flex items-center justify-between gap-3 px-4 py-3 transition-colors duration-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/30"
          >
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-medium text-neutral-950 dark:text-neutral-50">
                {{ cred.name }}
              </p>
              <p class="text-xs text-neutral-500 dark:text-neutral-400">
                {{ typeLabel(cred.type) }} · Updated {{ formatDate(cred.updatedAt) }}
              </p>
            </div>
            <div class="flex items-center gap-2">
              <StatusBadge
                status="success"
                label="Configured"
                dot
              />
              <div class="flex items-center gap-1">
                <IconButton
                  label="Edit credential"
                  @click="openEditModal(cred)"
                >
                  <PencilIcon class="h-4 w-4" />
                </IconButton>
                <IconButton
                  label="Delete credential"
                  tone="danger"
                  @click="deleteCredential(cred)"
                >
                  <TrashIcon class="h-4 w-4" />
                </IconButton>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Create / Edit modal -->
    <TransitionRoot :show="showModal" as="template">
      <Dialog @close="closeModal" class="relative z-50">
        <TransitionChild
          enter="ease-out duration-200"
          enter-from="opacity-0"
          enter-to="opacity-100"
          leave="ease-in duration-200"
          leave-from="opacity-100"
          leave-to="opacity-0"
        >
          <div class="fixed inset-0 bg-[oklch(100%_0_89.88/0.8)] backdrop-blur-[2px] dark:bg-black/60" />
        </TransitionChild>

        <div class="fixed inset-0 flex items-center justify-center p-4">
          <TransitionChild
            enter="ease-out duration-200"
            enter-from="opacity-0 scale-95"
            enter-to="opacity-100 scale-100"
            leave="ease-in duration-200"
            leave-from="opacity-100 scale-100"
            leave-to="opacity-0 scale-95"
          >
            <DialogPanel
              :ref="setPanelRef"
              class="max-h-[90vh] w-full max-w-xl overflow-y-auto overscroll-contain rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900"
              @scroll="onPanelScroll"
            >
              <div class="p-6">
                <DialogTitle class="text-xl font-semibold text-neutral-950 dark:text-neutral-50">
                  {{ editingCredential ? 'Edit credential' : 'New credential' }}
                </DialogTitle>

                <form
                  class="mt-4 space-y-4"
                  @submit.prevent="saveCredential"
                >
                  <!-- Credential name (internal label) -->
                  <div>
                    <label
                      for="credential-name"
                      class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300"
                    >
                      Credential name
                    </label>
                    <input
                      id="credential-name"
                      v-model="form.name"
                      type="text"
                      :class="CONTROL_CLASS"
                      placeholder="My Postgres prod"
                      required
                    >
                  </div>

                  <!-- Type chooser -->
                  <div v-if="!editingCredential">
                    <label
                      for="credential-type"
                      class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300"
                    >
                      Type
                    </label>
                    <select
                      id="credential-type"
                      v-model="form.type"
                      :class="CONTROL_CLASS"
                      required
                    >
                      <option value="" disabled>Select a credential type…</option>
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
                    <span class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
                      Type
                    </span>
                    <div
                      class="flex h-10 cursor-not-allowed items-center rounded-lg border border-neutral-200 bg-neutral-50 px-3 text-sm text-neutral-700 dark:border-neutral-800 dark:bg-neutral-800/40 dark:text-neutral-300"
                    >
                      {{ typeLabel(form.type) }}
                      <span class="ml-2 text-xs text-neutral-400">({{ form.type }})</span>
                    </div>
                    <p class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                      Credential type cannot be changed after creation. Create a new credential to switch types.
                    </p>
                  </div>

                  <!-- Schema-driven fields -->
                  <div
                    v-if="currentSchema && form.type"
                    class="space-y-4 border-t border-neutral-200 pt-4 dark:border-neutral-800"
                  >
                    <p
                      v-if="loadingCredential"
                      class="text-xs text-neutral-500 dark:text-neutral-400"
                    >
                      Loading stored values…
                    </p>
                    <p
                      v-else-if="editingCredential"
                      class="rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-800 dark:border-blue-900 dark:bg-blue-950/50 dark:text-blue-300"
                    >
                      Stored values are loaded below — edit any field directly and
                      save. Leave a password field untouched to keep it as-is.
                    </p>

                    <div
                      v-for="prop in currentSchema.properties"
                      :key="prop.name"
                    >
                      <label
                        class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300"
                      >
                        {{ prop.displayName }}
                        <span
                          v-if="currentSchema.required.includes(prop.name)"
                          class="text-red-600 dark:text-red-400"
                        >*</span>
                      </label>

                      <!-- String input -->
                      <input
                        v-if="prop.type === 'string'"
                        type="text"
                        :value="fieldValue(prop)"
                        :class="CONTROL_CLASS"
                        :placeholder="prop.placeholder ?? ''"
                        @input="updateData(prop.name, ($event.target as HTMLInputElement).value)"
                      >

                      <!-- Password input (with reveal toggle) -->
                      <div
                        v-else-if="prop.type === 'password'"
                        class="relative"
                      >
                        <input
                          :type="isRevealed(prop.name) ? 'text' : 'password'"
                          autocomplete="off"
                          :value="fieldValue(prop)"
                          :class="[CONTROL_CLASS, 'pr-10']"
                          :placeholder="prop.placeholder ?? ''"
                          @input="updateData(prop.name, ($event.target as HTMLInputElement).value)"
                        >
                        <button
                          type="button"
                          class="absolute inset-y-0 right-0 flex items-center px-3 text-neutral-400 transition-colors hover:text-neutral-700 dark:hover:text-neutral-200"
                          :title="isRevealed(prop.name) ? 'Hide value' : 'Reveal value'"
                          @click="toggleReveal(prop.name)"
                        >
                          <EyeSlashIcon
                            v-if="isRevealed(prop.name)"
                            class="h-4 w-4"
                          />
                          <EyeIcon
                            v-else
                            class="h-4 w-4"
                          />
                        </button>
                      </div>

                      <!-- Number input -->
                      <input
                        v-else-if="prop.type === 'number'"
                        type="number"
                        :value="fieldValue(prop)"
                        :class="CONTROL_CLASS"
                        @input="updateNumberField(prop.name, ($event.target as HTMLInputElement).value)"
                      >

                      <!-- Boolean toggle -->
                      <label
                        v-else-if="prop.type === 'boolean'"
                        class="flex items-center gap-2"
                      >
                        <input
                          type="checkbox"
                          :checked="!!fieldValue(prop)"
                          class="h-4 w-4 rounded border-neutral-300 text-brand-600 focus:ring-brand-500 dark:border-neutral-600 dark:bg-neutral-800"
                          @change="updateData(prop.name, ($event.target as HTMLInputElement).checked)"
                        >
                        <span class="text-sm text-neutral-600 dark:text-neutral-300">
                          {{ prop.description || prop.displayName }}
                        </span>
                      </label>

                      <!-- Options select -->
                      <select
                        v-else-if="prop.type === 'options' && prop.options"
                        :value="fieldValue(prop)"
                        :class="CONTROL_CLASS"
                        @change="updateData(prop.name, ($event.target as HTMLSelectElement).value)"
                      >
                        <option
                          v-for="opt in optionValues(prop)"
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
                        :class="[CONTROL_CLASS, 'font-mono']"
                        rows="4"
                        @input="updateJsonField(prop.name, ($event.target as HTMLTextAreaElement).value)"
                      />

                      <p
                        v-if="prop.description"
                        class="mt-1 text-xs text-neutral-500 dark:text-neutral-400"
                      >
                        {{ prop.description }}
                      </p>
                    </div>

                    <!-- Stored fields the schema does not describe
                         (e.g. rabbitmq's hostname / username). Keep them
                         visible and editable. -->
                    <div
                      v-if="extraDataFields.length > 0"
                      class="space-y-4 border-t border-neutral-100 pt-3 dark:border-neutral-800"
                    >
                      <p class="text-xs text-neutral-500 dark:text-neutral-400">
                        Additional stored fields
                      </p>
                      <div
                        v-for="key in extraDataFields"
                        :key="key"
                      >
                        <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
                          {{ key }}
                        </label>
                        <div
                          v-if="isSecretField(key)"
                          class="relative"
                        >
                          <input
                            :type="isRevealed(key) ? 'text' : 'password'"
                            autocomplete="off"
                            :value="String(form.data[key] ?? '')"
                            :class="[CONTROL_CLASS, 'pr-10']"
                            @input="updateData(key, ($event.target as HTMLInputElement).value)"
                          >
                          <button
                            type="button"
                            class="absolute inset-y-0 right-0 flex items-center px-3 text-neutral-400 transition-colors hover:text-neutral-700 dark:hover:text-neutral-200"
                            :title="isRevealed(key) ? 'Hide value' : 'Reveal value'"
                            @click="toggleReveal(key)"
                          >
                            <EyeSlashIcon
                              v-if="isRevealed(key)"
                              class="h-4 w-4"
                            />
                            <EyeIcon
                              v-else
                              class="h-4 w-4"
                            />
                          </button>
                        </div>
                        <input
                          v-else
                          type="text"
                          :value="String(form.data[key] ?? '')"
                          :class="CONTROL_CLASS"
                          @input="updateData(key, ($event.target as HTMLInputElement).value)"
                        >
                      </div>
                    </div>
                  </div>

                  <!-- Fallback for credential types without a form schema:
                       list the stored fields so they stay editable. -->
                  <div
                    v-else-if="!schemaLoading && form.type && Object.keys(form.data).length > 0"
                    class="space-y-4 border-t border-neutral-200 pt-4 dark:border-neutral-800"
                  >
                    <p class="text-xs text-neutral-500 dark:text-neutral-400">
                      This credential type has no form schema — the stored fields
                      are listed below and can be edited directly.
                    </p>
                    <div
                      v-for="(value, key) in form.data"
                      :key="String(key)"
                    >
                      <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
                        {{ key }}
                      </label>
                      <div
                        v-if="isSecretField(String(key))"
                        class="relative"
                      >
                        <input
                          :type="isRevealed(String(key)) ? 'text' : 'password'"
                          autocomplete="off"
                          :value="value"
                          :class="[CONTROL_CLASS, 'pr-10']"
                          @input="updateData(String(key), ($event.target as HTMLInputElement).value)"
                        >
                        <button
                          type="button"
                          class="absolute inset-y-0 right-0 flex items-center px-3 text-neutral-400 transition-colors hover:text-neutral-700 dark:hover:text-neutral-200"
                          :title="isRevealed(String(key)) ? 'Hide value' : 'Reveal value'"
                          @click="toggleReveal(String(key))"
                        >
                          <EyeSlashIcon
                            v-if="isRevealed(String(key))"
                            class="h-4 w-4"
                          />
                          <EyeIcon
                            v-else
                            class="h-4 w-4"
                          />
                        </button>
                      </div>
                      <input
                        v-else
                        type="text"
                        :value="value"
                        :class="CONTROL_CLASS"
                        @input="updateData(String(key), ($event.target as HTMLInputElement).value)"
                      >
                    </div>
                  </div>

                  <!-- Inline test result -->
                  <div
                    v-if="testResult"
                    class="border-t border-neutral-200 pt-4 dark:border-neutral-800"
                  >
                    <div
                      class="flex items-start gap-2 rounded-lg border px-3 py-2 text-sm"
                      :class="testResult.status === 'OK'
                        ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/50 dark:text-green-300'
                        : 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300'"
                    >
                      <CheckCircleIcon
                        v-if="testResult.status === 'OK'"
                        class="mt-0.5 h-4 w-4 flex-shrink-0"
                      />
                      <XCircleIcon
                        v-else
                        class="mt-0.5 h-4 w-4 flex-shrink-0"
                      />
                      <div>
                        <div class="font-semibold">{{ testResult.status }}</div>
                        <div>{{ testResult.message }}</div>
                      </div>
                    </div>
                  </div>

                  <!-- Missing required warning -->
                  <p
                    v-if="requiredFieldsMissing.length > 0"
                    class="text-xs text-yellow-800 dark:text-yellow-300"
                  >
                    Missing required fields: {{ requiredFieldsMissing.join(', ') }}
                  </p>

                  <!-- Action row -->
                  <div class="flex items-center justify-between border-t border-neutral-200 pt-4 dark:border-neutral-800">
                    <Button
                      variant="secondary"
                      size="sm"
                      :disabled="testing || !form.type"
                      @click="runTest"
                    >
                      {{ testing ? 'Testing…' : 'Test connection' }}
                    </Button>

                    <div class="flex gap-2">
                      <Button
                        variant="secondary"
                        size="sm"
                        @click="closeModal"
                      >
                        Cancel
                      </Button>
                      <Button
                        type="submit"
                        variant="primary"
                        size="sm"
                      >
                        {{ editingCredential ? 'Save' : 'Create' }}
                      </Button>
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
