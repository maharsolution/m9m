<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
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
} from '@heroicons/vue/24/outline'
import { Menu, MenuButton, MenuItems, MenuItem } from '@headlessui/vue'
import { useWorkflowEditorStore, useWorkflowStore } from '@/stores'
import type { Workflow } from '@/types'
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

const searchQuery = ref('')
const activeFilter = ref<'all' | 'active' | 'inactive'>('all')

// Selection state for the bulk export. The ids are kept in a Set so
// the toggle is O(1) and the selection survives a list refresh.
const selectedIds = ref<Set<string>>(new Set())
const isExporting = ref(false)
const isImporting = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const notice = ref<{ type: 'success' | 'error'; message: string; details: string[] } | null>(null)

onMounted(async () => {
  await workflowStore.fetchWorkflows()
})

const selectedCount = computed(() => selectedIds.value.size)

const filteredWorkflows = computed(() => {
  let workflows = workflowStore.workflows

  // Filter by active status
  if (activeFilter.value === 'active') {
    workflows = workflows.filter((w) => w.active)
  } else if (activeFilter.value === 'inactive') {
    workflows = workflows.filter((w) => !w.active)
  }

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
  <div class="p-6">
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <div>
        <h1 class="text-2xl font-bold text-slate-900 dark:text-white">Workflows</h1>
        <p class="text-slate-500 dark:text-slate-400">
          Manage and run your automation workflows
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button
          @click="triggerImport"
          :disabled="isImporting"
          class="btn-secondary flex items-center gap-2"
        >
          <ArrowUpTrayIcon class="w-5 h-5" />
          <span>{{ isImporting ? 'Importing...' : 'Import' }}</span>
        </button>
        <button
          @click="exportSelectedWorkflows"
          :disabled="selectedCount === 0 || isExporting"
          class="btn-secondary flex items-center gap-2"
          :title="selectedCount === 0 ? 'Select at least one workflow to export' : 'Export the selected workflows'"
        >
          <ArrowDownTrayIcon class="w-5 h-5" />
          <span>Export selected<template v-if="selectedCount > 0"> ({{ selectedCount }})</template></span>
        </button>
        <button @click="createWorkflow" class="btn-primary flex items-center gap-2">
          <PlusIcon class="w-5 h-5" />
          <span>New Workflow</span>
        </button>
      </div>
    </div>

    <!-- Hidden file picker for the n8n JSON import -->
    <input
      ref="fileInput"
      type="file"
      accept=".json,application/json"
      class="hidden"
      @change="handleImportFile"
    />

    <!-- Import / export feedback -->
    <div
      v-if="notice"
      :class="notice.type === 'success'
        ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-300'
        : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-800 dark:text-red-300'"
      class="mb-4 flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
    >
      <CheckCircleIcon v-if="notice.type === 'success'" class="w-5 h-5 shrink-0 mt-0.5" />
      <ExclamationTriangleIcon v-else class="w-5 h-5 shrink-0 mt-0.5" />
      <div class="flex-1 min-w-0">
        <p>{{ notice.message }}</p>
        <ul v-if="notice.details.length > 0" class="mt-1 list-disc list-inside space-y-0.5">
          <li v-for="(detail, index) in notice.details" :key="index" class="break-words">
            {{ detail }}
          </li>
        </ul>
      </div>
      <button @click="notice = null" class="shrink-0 opacity-60 hover:opacity-100">
        <XMarkIcon class="w-4 h-4" />
      </button>
    </div>

    <!-- Filters -->
    <div class="flex items-center gap-4 mb-6">
      <!-- Search -->
      <div class="relative flex-1 max-w-md">
        <MagnifyingGlassIcon class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-slate-400" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search workflows..."
          class="input pl-10"
        />
      </div>

      <!-- Status Filter -->
      <div class="flex items-center gap-2 bg-slate-100 dark:bg-slate-800 rounded-lg p-1">
        <button
          v-for="filter in [
            { value: 'all', label: 'All' },
            { value: 'active', label: 'Active' },
            { value: 'inactive', label: 'Inactive' },
          ]"
          :key="filter.value"
          @click="activeFilter = filter.value as typeof activeFilter"
          :class="[
            'px-3 py-1.5 text-sm font-medium rounded-md transition-colors',
            activeFilter === filter.value
              ? 'bg-white dark:bg-slate-700 text-slate-900 dark:text-white shadow-sm'
              : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-white'
          ]"
        >
          {{ filter.label }}
        </button>
      </div>
    </div>

    <!-- Workflow Grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      <div
        v-for="workflow in filteredWorkflows"
        :key="workflow.id"
        @click="openWorkflow(workflow)"
        class="card-hover p-4"
      >
        <div class="flex items-start justify-between">
          <div class="flex items-center gap-3">
            <input
              type="checkbox"
              :checked="isSelected(workflow.id)"
              @click.stop
              @change="toggleSelection(workflow.id)"
              class="mt-1 h-4 w-4 rounded border-slate-300 text-primary-600 focus:ring-primary-500 dark:bg-slate-800 dark:border-slate-600"
              :aria-label="`Select ${workflow.name}`"
            />
            <div :class="[
              workflow.active
                ? 'bg-green-100 dark:bg-green-900/30'
                : 'bg-slate-100 dark:bg-slate-700',
              'p-3 rounded-lg'
            ]">
              <BoltIcon :class="[
                workflow.active
                  ? 'text-green-600 dark:text-green-400'
                  : 'text-slate-500 dark:text-slate-400',
                'w-6 h-6'
              ]" />
            </div>
            <div>
              <h3 class="font-semibold text-slate-900 dark:text-white">
                {{ workflow.name }}
              </h3>
              <p class="text-sm text-slate-500 dark:text-slate-400">
                {{ workflow.nodes.length }} nodes
              </p>
            </div>
          </div>

          <!-- Actions Menu -->
          <Menu as="div" class="relative">
            <MenuButton
              @click.stop
              class="p-1 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700"
            >
              <EllipsisVerticalIcon class="w-5 h-5" />
            </MenuButton>

            <transition
              enter-active-class="transition duration-100 ease-out"
              enter-from-class="transform scale-95 opacity-0"
              enter-to-class="transform scale-100 opacity-100"
              leave-active-class="transition duration-75 ease-in"
              leave-from-class="transform scale-100 opacity-100"
              leave-to-class="transform scale-95 opacity-0"
            >
              <MenuItems class="absolute right-0 mt-1 w-48 origin-top-right bg-white dark:bg-slate-800 rounded-lg shadow-lg ring-1 ring-black ring-opacity-5 dark:ring-slate-700 focus:outline-none py-1 z-10">
                <MenuItem v-slot="{ active }">
                  <button
                    @click.stop="executeWorkflow(workflow, $event)"
                    class="flex items-center gap-2 w-full px-4 py-2 text-sm text-slate-700 dark:text-slate-200"
                    :class="{ 'bg-slate-100 dark:bg-slate-700': active }"
                  >
                    <PlayIcon class="w-4 h-4" />
                    Execute
                  </button>
                </MenuItem>
                <MenuItem v-slot="{ active }">
                  <button
                    @click.stop="toggleActive(workflow, $event)"
                    class="flex items-center gap-2 w-full px-4 py-2 text-sm text-slate-700 dark:text-slate-200"
                    :class="{ 'bg-slate-100 dark:bg-slate-700': active }"
                  >
                    <component :is="workflow.active ? StopIcon : PlayIcon" class="w-4 h-4" />
                    {{ workflow.active ? 'Deactivate' : 'Activate' }}
                  </button>
                </MenuItem>
                <MenuItem v-slot="{ active }">
                  <button
                    @click.stop="duplicateWorkflow(workflow)"
                    class="flex items-center gap-2 w-full px-4 py-2 text-sm text-slate-700 dark:text-slate-200"
                    :class="{ 'bg-slate-100 dark:bg-slate-700': active }"
                  >
                    <DocumentDuplicateIcon class="w-4 h-4" />
                    Duplicate
                  </button>
                </MenuItem>
                <MenuItem v-slot="{ active }">
                  <button
                    @click.stop="exportSingleWorkflow(workflow, $event)"
                    :disabled="isExporting"
                    class="flex items-center gap-2 w-full px-4 py-2 text-sm text-slate-700 dark:text-slate-200 disabled:opacity-50"
                    :class="{ 'bg-slate-100 dark:bg-slate-700': active }"
                  >
                    <ArrowDownTrayIcon class="w-4 h-4" />
                    Export
                  </button>
                </MenuItem>
                <div class="border-t border-slate-200 dark:border-slate-700 my-1" />
                <MenuItem v-slot="{ active }">
                  <button
                    @click.stop="deleteWorkflow(workflow)"
                    class="flex items-center gap-2 w-full px-4 py-2 text-sm text-red-600 dark:text-red-400"
                    :class="{ 'bg-red-50 dark:bg-red-900/20': active }"
                  >
                    <TrashIcon class="w-4 h-4" />
                    Delete
                  </button>
                </MenuItem>
              </MenuItems>
            </transition>
          </Menu>
        </div>

        <p v-if="workflow.description" class="mt-3 text-sm text-slate-600 dark:text-slate-300 line-clamp-2">
          {{ workflow.description }}
        </p>

        <div class="mt-4 flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
          <span>Updated {{ formatDate(workflow.updatedAt) }}</span>
          <span :class="workflow.active ? 'badge-success' : 'badge-info'">
            {{ workflow.active ? 'Active' : 'Inactive' }}
          </span>
        </div>
      </div>
    </div>

    <!-- Empty State -->
    <div
      v-if="filteredWorkflows.length === 0 && !workflowStore.loading"
      class="text-center py-16"
    >
      <BoltIcon class="w-16 h-16 mx-auto text-slate-300 dark:text-slate-600" />
      <h3 class="mt-4 text-lg font-medium text-slate-900 dark:text-white">
        {{ searchQuery ? 'No workflows found' : 'No workflows yet' }}
      </h3>
      <p class="mt-2 text-slate-500 dark:text-slate-400">
        {{ searchQuery ? 'Try a different search term' : 'Get started by creating your first workflow' }}
      </p>
      <button
        v-if="!searchQuery"
        @click="createWorkflow"
        class="mt-6 btn-primary"
      >
        Create Workflow
      </button>
    </div>

    <!-- Loading State -->
    <div v-if="workflowStore.loading" class="text-center py-16">
      <div class="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto" />
      <p class="mt-4 text-slate-500 dark:text-slate-400">Loading workflows...</p>
    </div>
  </div>
</template>
