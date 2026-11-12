<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  MagnifyingGlassIcon,
  PlusIcon,
  BoltIcon,
  PlayIcon,
  StopIcon,
  TrashIcon,
  DocumentDuplicateIcon,
  EllipsisVerticalIcon,
  ArrowDownTrayIcon,
  ArrowUpTrayIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  XMarkIcon,
  TagIcon,
} from '@heroicons/vue/24/outline'
import { Menu, MenuButton, MenuItems, MenuItem } from '@headlessui/vue'
import { useWorkflowEditorStore, useWorkflowStore } from '@/stores'
import { useTagsStore } from '@/stores/tags'
import type { Workflow } from '@/types'
import type { Tag } from '@/types/tag'
import TagChip from '@/components/tags/TagChip.vue'
import TagsModal from '@/components/tags/TagsModal.vue'
import Button from '@/components/common/Button.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import DataTable, { type DataTableColumn } from '@/components/common/DataTable.vue'
import {
  apiErrorDetails,
  apiErrorMessage,
  downloadWorkflowExport,
  exportWorkflow,
  exportWorkflows,
  importWorkflowFile,
} from '@/api'

const router = useRouter()
const workflowStore = useWorkflowStore()
const workflowEditorStore = useWorkflowEditorStore()
const tagsStore = useTagsStore()

const searchQuery = ref('')
const activeFilter = ref<'all' | 'active' | 'inactive'>('all')
// Tag filter. The list endpoint accepts the selected tag ID through the
// `tags` query parameter, so the list is re-fetched whenever the
// selection changes.
const selectedTagId = ref<string | null>(null)
// Offset of the requested page. A new filter starts a new result set,
// so the offset is reset whenever the tag filter changes.
const listOffset = ref(0)
const showTagsModal = ref(false)

// Selection state for the bulk export. The ids are kept in a Set so
// the toggle is O(1) and the selection survives a list refresh.
const selectedIds = ref<Set<string>>(new Set())
const isExporting = ref(false)
const isImporting = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const notice = ref<{ type: 'success' | 'error'; message: string; details: string[] } | null>(null)

const statusFilters = [
  { value: 'all', label: 'All' },
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' },
] as const

/** Columns of the n8n-style workflow table. */
const workflowColumns: DataTableColumn[] = [
  { key: 'select', label: '', width: '40px' },
  { key: 'name', label: 'Name' },
  { key: 'status', label: 'Status', width: '112px' },
  { key: 'tags', label: 'Tags' },
  { key: 'updated', label: 'Updated', width: '144px' },
  { key: 'actions', label: 'Actions', width: '48px', align: 'right' },
]

/**
 * Load the workflow list with the currently selected tag filter.
 *
 * The tag filter is applied by the server (`?tags=<id>`), so the page
 * holds the matching workflows only and the server side total stays
 * correct.
 */
async function loadWorkflows() {
  await workflowStore.fetchWorkflows({
    limit: 100,
    offset: listOffset.value,
    tags: selectedTagId.value ? [selectedTagId.value] : undefined,
  })
}

onMounted(async () => {
  await loadWorkflows()
  try {
    await tagsStore.fetchAllTags()
  } catch (e) {
    console.error('Failed to load tags:', e)
  }
})

// Re-fetch whenever the tag filter changes so the server does the
// matching instead of the client filtering a truncated page. The offset
// is reset first: the previous page number belongs to the unfiltered
// list and would skip rows of the new result set.
watch(selectedTagId, () => {
  listOffset.value = 0
  void loadWorkflows()
})

const selectedCount = computed(() => selectedIds.value.size)

const availableTags = computed(() => tagsStore.sortedTags)

/**
 * The tag chips of every workflow. The workflow document only holds the
 * tag IDs, so they are resolved against the loaded tag list.
 */
const workflowChips = computed<Record<string, Tag[]>>(() => {
  const map: Record<string, Tag[]> = {}
  // The API answers with `null` instead of an empty list when nothing
  // matches the filter.
  for (const workflow of workflowStore.workflows ?? []) {
    map[workflow.id] = (workflow.tags ?? [])
      .map((id) => tagsStore.tagMap[id])
      .filter((tag): tag is Tag => !!tag)
  }
  return map
})

const filteredWorkflows = computed(() => {
  let workflows = workflowStore.workflows ?? []

  // Filter by active status
  if (activeFilter.value === 'active') {
    workflows = workflows.filter((w) => w.active)
  } else if (activeFilter.value === 'inactive') {
    workflows = workflows.filter((w) => !w.active)
  }

  // The tag filter is applied by the server: the fetched list already
  // contains the matching workflows only, so filtering it again here
  // would hide workflows that do match.

  // Filter by search query
  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    workflows = workflows.filter(
      (w) =>
        w.name.toLowerCase().includes(query) ||
        w.description?.toLowerCase().includes(query)
    )
  }

  return workflows
})

const isSelected = (id: string) => selectedIds.value.has(id)

const toggleSelection = (id: string) => {
  const next = new Set(selectedIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedIds.value = next
}

/** Every visible row is selected — drives the header checkbox. */
const allSelected = computed(
  () => filteredWorkflows.value.length > 0 && filteredWorkflows.value.every((w) => selectedIds.value.has(w.id))
)

/** Select or clear every visible row. */
const toggleSelectAll = () => {
  const next = new Set(selectedIds.value)
  if (allSelected.value) {
    for (const workflow of filteredWorkflows.value) next.delete(workflow.id)
  } else {
    for (const workflow of filteredWorkflows.value) next.add(workflow.id)
  }
  selectedIds.value = next
}

const clearSelection = () => {
  selectedIds.value = new Set()
}

const openWorkflow = (workflow: Workflow) => {
  router.push(`/workflows/${workflow.id}`)
}

const createWorkflow = () => {
  router.push('/workflows/new')
}

const toggleActive = async (workflow: Workflow, event: Event) => {
  event.stopPropagation()
  await workflowStore.toggleWorkflowActive(workflow.id)
}

const duplicateWorkflow = async (workflow: Workflow) => {
  try {
    workflowEditorStore.createNewWorkflow(workflow, {
      id: '',
      name: `${workflow.name} (copy)`,
      active: false,
    })
    router.push('/workflows/new')
  } catch (e) {
    console.error('Failed to duplicate workflow:', e)
  }
}

const deleteWorkflow = async (workflow: Workflow) => {
  if (confirm(`Are you sure you want to delete "${workflow.name}"?`)) {
    await workflowStore.deleteWorkflow(workflow.id)
  }
}

const executeWorkflow = async (workflow: Workflow, event: Event) => {
  event.stopPropagation()
  try {
    const execution = await workflowStore.executeWorkflow(workflow.id)
    router.push(`/executions/${execution.id}`)
  } catch (e) {
    console.error('Failed to execute workflow:', e)
  }
}

// Export a single workflow (n8n editor "Download" parity): the
// server answers with one workflow object.
const exportSingleWorkflow = async (workflow: Workflow, event: Event) => {
  event.stopPropagation()
  isExporting.value = true
  notice.value = null
  try {
    const file = await exportWorkflow(workflow.id)
    downloadWorkflowExport(file)
    notice.value = {
      type: 'success',
      message: `Exported "${workflow.name}" as ${file.filename}.`,
      details: [],
    }
  } catch (e) {
    notice.value = {
      type: 'error',
      message: await apiErrorMessage(e, 'Failed to export the workflow.'),
      details: [],
    }
  } finally {
    isExporting.value = false
  }
}

// Export the selected workflows. The server always answers with an
// ARRAY of workflow objects (n8n CLI `export:workflow` parity), also
// when a single workflow is selected.
const exportSelectedWorkflows = async () => {
  const ids = Array.from(selectedIds.value)
  if (ids.length === 0) return

  isExporting.value = true
  notice.value = null
  try {
    const file = await exportWorkflows(ids)
    downloadWorkflowExport(file)
    notice.value = {
      type: 'success',
      message: `Exported ${ids.length} workflow${ids.length === 1 ? '' : 's'} as ${file.filename}.`,
      details: [],
    }
    clearSelection()
  } catch (e) {
    notice.value = {
      type: 'error',
      message: await apiErrorMessage(e, 'Failed to export the workflows.'),
      details: [],
    }
  } finally {
    isExporting.value = false
  }
}

// Import an n8n workflow file. The file may hold one workflow or an
// array of workflows; the server reports per-item errors and warnings.
const triggerImport = () => {
  fileInput.value?.click()
}

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  // Reset so the same file can be picked again.
  input.value = ''
  if (!file) return

  isImporting.value = true
  notice.value = null
  try {
    const summary = await importWorkflowFile(file)

    const details: string[] = []
    for (const item of summary.workflows) {
      for (const warning of item.warnings ?? []) {
        details.push(`${item.name}: ${warning}`)
      }
    }
    for (const err of summary.errors) {
      details.push(`${err.name || `Entry #${err.index + 1}`}: ${err.message}`)
    }

    if (summary.failed > 0) {
      notice.value = {
        type: 'error',
        message: `Imported ${summary.imported} of ${summary.imported + summary.failed} workflows.`,
        details,
      }
    } else {
      notice.value = {
        type: 'success',
        message: `Imported ${summary.imported} workflow${summary.imported === 1 ? '' : 's'}.`,
        details,
      }
    }

    await workflowStore.fetchWorkflows()
  } catch (e) {
    notice.value = {
      type: 'error',
      message: await apiErrorMessage(e, 'Failed to import the workflow file.'),
      // The server repeats the per-item errors at the top level of a
      // rejected import, so the rejected entries are visible too.
      details: await apiErrorDetails(e),
    }
  } finally {
    isImporting.value = false
  }
}

const formatDate = (dateStr: string) => {
  const date = new Date(dateStr)
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
</script>

<template>
  <div>
    <!-- Page header -->
    <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
          Workflows
        </h1>
        <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
          Manage and run your automation workflows
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          :disabled="isImporting"
          :loading="isImporting"
          @click="triggerImport"
        >
          <template #icon>
            <ArrowUpTrayIcon class="h-4 w-4" />
          </template>
          <span>{{ isImporting ? 'Importing...' : 'Import' }}</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          :disabled="selectedCount === 0 || isExporting"
          :loading="isExporting"
          :title="selectedCount === 0 ? 'Select at least one workflow to export' : 'Export the selected workflows'"
          @click="exportSelectedWorkflows"
        >
          <template #icon>
            <ArrowDownTrayIcon class="h-4 w-4" />
          </template>
          <span>Export selected<template v-if="selectedCount > 0"> ({{ selectedCount }})</template></span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          title="Create, rename and delete tags"
          @click="showTagsModal = true"
        >
          <template #icon>
            <TagIcon class="h-4 w-4" />
          </template>
          <span>Manage tags</span>
        </Button>
        <Button
          variant="primary"
          size="sm"
          @click="createWorkflow"
        >
          <template #icon>
            <PlusIcon class="h-4 w-4" />
          </template>
          <span>New workflow</span>
        </Button>
      </div>
    </div>

    <!-- Hidden file picker for the n8n JSON import -->
    <input
      ref="fileInput"
      type="file"
      accept=".json,application/json"
      class="hidden"
      @change="handleImportFile"
    >

    <!-- Import / export feedback -->
    <div
      v-if="notice"
      class="mb-4 flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
      :class="notice.type === 'success'
        ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/50 dark:text-green-300'
        : 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300'"
    >
      <CheckCircleIcon
        v-if="notice.type === 'success'"
        class="mt-0.5 h-4 w-4 shrink-0"
      />
      <ExclamationTriangleIcon
        v-else
        class="mt-0.5 h-4 w-4 shrink-0"
      />
      <div class="min-w-0 flex-1">
        <p>{{ notice.message }}</p>
        <ul
          v-if="notice.details.length > 0"
          class="mt-1 list-inside list-disc space-y-0.5"
        >
          <li
            v-for="(detail, index) in notice.details"
            :key="index"
            class="break-words"
          >
            {{ detail }}
          </li>
        </ul>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-sm opacity-60 transition-opacity hover:opacity-100 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500"
        aria-label="Dismiss message"
        @click="notice = null"
      >
        <XMarkIcon class="h-4 w-4" />
      </button>
    </div>

    <!-- Toolbar: search + status filter -->
    <div class="mb-4 flex flex-wrap items-center gap-3">
      <div class="relative w-full max-w-sm">
        <MagnifyingGlassIcon class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-neutral-400" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search workflows…"
          class="h-9 w-full rounded-lg border border-neutral-300 bg-white pl-9 pr-3 text-sm text-neutral-950 transition-colors duration-200 placeholder:text-neutral-400 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10"
        >
      </div>

      <!-- Status Filter -->
      <div class="inline-flex items-center gap-1 rounded-lg border border-neutral-200 bg-neutral-100 p-1 dark:border-neutral-800 dark:bg-neutral-950">
        <button
          v-for="filter in statusFilters"
          :key="filter.value"
          type="button"
          @click="activeFilter = filter.value"
          class="rounded-md px-3 py-1 text-xs font-medium transition-colors duration-200"
          :class="activeFilter === filter.value
            ? 'bg-white text-neutral-950 shadow-xs dark:bg-neutral-800 dark:text-neutral-50'
            : 'text-neutral-600 hover:text-neutral-950 dark:text-neutral-400 dark:hover:text-neutral-100'"
        >
          {{ filter.label }}
        </button>
      </div>
    </div>

    <!-- Tag Filter. The selection is sent to the list endpoint through the
         `tags` query parameter, so the matching happens on the server. -->
    <div
      v-if="availableTags.length > 0"
      class="mb-4 flex flex-wrap items-center gap-2"
    >
      <span class="text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
        Tags
      </span>
      <button
        type="button"
        @click="selectedTagId = null"
        :aria-pressed="selectedTagId === null"
        class="inline-flex h-5 items-center rounded-lg border px-2 text-2xs font-medium transition-colors duration-200"
        :class="selectedTagId === null
          ? 'border-brand-200 bg-brand-50 text-brand-700 dark:border-brand-900 dark:bg-brand-950/50 dark:text-brand-300'
          : 'border-neutral-200 bg-white text-neutral-600 hover:bg-neutral-50 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-300 dark:hover:bg-neutral-800'"
      >
        All
      </button>
      <button
        v-for="tag in availableTags"
        :key="tag.id"
        type="button"
        :title="`Show only workflows tagged ${tag.name}`"
        :aria-pressed="selectedTagId === tag.id"
        class="rounded-lg transition-transform hover:scale-105"
        @click="selectedTagId = selectedTagId === tag.id ? null : tag.id"
      >
        <TagChip
          :tag="tag"
          :class="selectedTagId === tag.id ? 'ring-2 ring-brand-500 ring-offset-1 dark:ring-offset-neutral-950' : ''"
        />
      </button>
    </div>

    <!-- Loading State -->
    <div
      v-if="workflowStore.loading"
      class="flex flex-col items-center justify-center rounded-lg border border-neutral-200 bg-white py-16 dark:border-neutral-800 dark:bg-neutral-900"
    >
      <span
        class="inline-block h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
        aria-hidden="true"
      />
      <p class="mt-3 text-sm text-neutral-600 dark:text-neutral-400">
        Loading workflows…
      </p>
    </div>

    <!-- Workflow table (n8n parity) -->
    <DataTable
      v-else
      :columns="workflowColumns"
      :rows="filteredWorkflows"
      row-key="id"
      clickable
      @row-click="openWorkflow"
    >
      <!-- Select-all checkbox -->
      <template #header-select>
        <input
          type="checkbox"
          :checked="allSelected"
          class="h-4 w-4 rounded border-neutral-300 text-brand-600 focus:ring-brand-500 dark:border-neutral-600 dark:bg-neutral-800"
          aria-label="Select all workflows"
          @click.stop
          @change="toggleSelectAll"
        >
      </template>

      <!-- Row selection -->
      <template #select="{ row }">
        <input
          type="checkbox"
          :checked="isSelected(row.id)"
          class="h-4 w-4 rounded border-neutral-300 text-brand-600 focus:ring-brand-500 dark:border-neutral-600 dark:bg-neutral-800"
          :aria-label="`Select ${row.name}`"
          @click.stop
          @change="toggleSelection(row.id)"
        >
      </template>

      <!-- Name -->
      <template #name="{ row }">
        <div class="flex items-center gap-3">
          <span
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
            :class="row.active
              ? 'bg-green-50 dark:bg-green-950/50'
              : 'bg-neutral-100 dark:bg-neutral-800'"
          >
            <BoltIcon
              class="h-4 w-4"
              :class="row.active
                ? 'text-green-600 dark:text-green-400'
                : 'text-neutral-500 dark:text-neutral-400'"
            />
          </span>
          <div class="min-w-0">
            <p class="truncate text-sm font-semibold text-neutral-950 dark:text-neutral-50">
              {{ row.name }}
            </p>
            <p class="truncate text-xs text-neutral-500 dark:text-neutral-400">
              {{ row.description || `${row.nodes.length} nodes` }}
            </p>
          </div>
        </div>
      </template>

      <!-- Status -->
      <template #status="{ row }">
        <StatusBadge
          :status="row.active ? 'success' : 'neutral'"
          :label="row.active ? 'Active' : 'Inactive'"
          dot
        />
      </template>

      <!-- Tags -->
      <template #tags="{ row }">
        <div
          v-if="(workflowChips[row.id] ?? []).length > 0"
          class="flex flex-wrap gap-1"
        >
          <TagChip
            v-for="tag in workflowChips[row.id]"
            :key="tag.id"
            :tag="tag"
          />
        </div>
        <span
          v-else
          class="text-xs text-neutral-400"
        >—</span>
      </template>

      <!-- Updated -->
      <template #updated="{ row }">
        <span class="whitespace-nowrap text-xs text-neutral-500 dark:text-neutral-400">
          {{ formatDate(row.updatedAt) }}
        </span>
      </template>

      <!-- Actions -->
      <template #actions="{ row }">
        <div
          class="flex justify-end"
          @click.stop
        >
          <Menu
            as="div"
            class="relative inline-block text-left"
          >
            <MenuButton
              class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-neutral-400 transition-colors duration-200 hover:bg-neutral-100 hover:text-neutral-700 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 dark:hover:bg-neutral-800 dark:hover:text-neutral-200 dark:focus-visible:outline-neutral-100"
            >
              <EllipsisVerticalIcon class="h-4 w-4" />
            </MenuButton>

            <transition
              enter-active-class="transition duration-100 ease-out"
              enter-from-class="transform scale-95 opacity-0"
              enter-to-class="transform scale-100 opacity-100"
              leave-active-class="transition duration-75 ease-in"
              leave-from-class="transform scale-100 opacity-100"
              leave-to-class="transform scale-95 opacity-0"
            >
              <MenuItems class="absolute right-0 z-20 mt-1 w-48 origin-top-right rounded-lg border border-neutral-200 bg-white py-1 shadow-md focus:outline-none dark:border-neutral-700 dark:bg-neutral-900">
                <MenuItem v-slot="{ active }">
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-neutral-700 dark:text-neutral-200"
                    :class="active ? 'bg-neutral-100 dark:bg-neutral-800' : ''"
                    @click.stop="executeWorkflow(row, $event)"
                  >
                    <PlayIcon class="h-4 w-4" />
                    Execute
                  </button>
                </MenuItem>
                <MenuItem v-slot="{ active }">
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-neutral-700 dark:text-neutral-200"
                    :class="active ? 'bg-neutral-100 dark:bg-neutral-800' : ''"
                    @click.stop="toggleActive(row, $event)"
                  >
                    <component
                      :is="row.active ? StopIcon : PlayIcon"
                      class="h-4 w-4"
                    />
                    {{ row.active ? 'Deactivate' : 'Activate' }}
                  </button>
                </MenuItem>
                <MenuItem v-slot="{ active }">
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-neutral-700 dark:text-neutral-200"
                    :class="active ? 'bg-neutral-100 dark:bg-neutral-800' : ''"
                    @click.stop="duplicateWorkflow(row)"
                  >
                    <DocumentDuplicateIcon class="h-4 w-4" />
                    Duplicate
                  </button>
                </MenuItem>
                <MenuItem v-slot="{ active }">
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-neutral-700 disabled:opacity-50 dark:text-neutral-200"
                    :class="active ? 'bg-neutral-100 dark:bg-neutral-800' : ''"
                    :disabled="isExporting"
                    @click.stop="exportSingleWorkflow(row, $event)"
                  >
                    <ArrowDownTrayIcon class="h-4 w-4" />
                    Export
                  </button>
                </MenuItem>
                <div class="my-1 border-t border-neutral-200 dark:border-neutral-800" />
                <MenuItem v-slot="{ active }">
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 px-4 py-2 text-left text-sm text-red-600 dark:text-red-400"
                    :class="active ? 'bg-red-50 dark:bg-red-950/40' : ''"
                    @click.stop="deleteWorkflow(row)"
                  >
                    <TrashIcon class="h-4 w-4" />
                    Delete
                  </button>
                </MenuItem>
              </MenuItems>
            </transition>
          </Menu>
        </div>
      </template>

      <!-- Empty state -->
      <template #empty>
        <div class="flex flex-col items-center px-4 py-16 text-center">
          <BoltIcon class="h-12 w-12 text-neutral-300 dark:text-neutral-700" />
          <h3 class="mt-4 text-lg font-semibold text-neutral-950 dark:text-neutral-100">
            {{ searchQuery || selectedTagId ? 'No workflows found' : 'No workflows yet' }}
          </h3>
          <p class="mt-1 max-w-sm text-sm text-neutral-600 dark:text-neutral-400">
            {{ searchQuery
              ? 'Try a different search term'
              : selectedTagId
                ? 'No workflow carries the selected tag'
                : 'Get started by creating your first workflow' }}
          </p>
          <Button
            v-if="!searchQuery && !selectedTagId"
            variant="primary"
            size="sm"
            class="mt-6"
            @click="createWorkflow"
          >
            Create workflow
          </Button>
          <Button
            v-else-if="selectedTagId"
            variant="secondary"
            size="sm"
            class="mt-6"
            @click="selectedTagId = null"
          >
            Clear the tag filter
          </Button>
        </div>
      </template>
    </DataTable>

    <!-- Global tag management -->
    <TagsModal
      v-if="showTagsModal"
      @close="showTagsModal = false"
    />
  </div>
</template>
