<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  PlayIcon,
  StopIcon,
  ArrowPathIcon,
  CloudArrowUpIcon,
  PencilIcon,
  SparklesIcon,
  BugAntIcon,
  ArrowDownTrayIcon,
  ArrowUturnLeftIcon,
  ArrowUturnRightIcon,
  Squares2X2Icon,
  TagIcon,
  ClockIcon,
  ChevronLeftIcon,
} from '@heroicons/vue/24/outline'
import type { Tag } from '@/types/tag'
import TagChip from '@/components/tags/TagChip.vue'

const props = withDefaults(defineProps<{
  workflowName?: string
  workflowActive?: boolean
  workflowDebug?: boolean
  isNewWorkflow: boolean
  isDirty: boolean
  isLoading: boolean
  isExecuting: boolean
  showNodePalette: boolean
  showAgentAi?: boolean
  isExporting?: boolean
  /** Whether an undo step is available. */
  canUndo?: boolean
  /** Whether a redo step is available. */
  canRedo?: boolean
  /** Tags currently attached to the workflow. */
  workflowTags?: Tag[]
}>(), {
  workflowName: '',
  workflowActive: false,
  workflowDebug: false,
  showAgentAi: false,
  isExporting: false,
  canUndo: false,
  canRedo: false,
  workflowTags: () => [],
})

const emit = defineEmits<{
  back: []
  rename: [name: string]
  togglePalette: []
  toggleAi: []
  toggleActive: []
  toggleDebug: []
  execute: []
  save: []
  export: []
  undo: []
  redo: []
  autoLayout: []
  addStickyNote: []
  showTags: []
  showVersions: []
}>()

const editingName = ref(false)
const workflowNameInput = ref('')

const displayName = computed(() => props.workflowName || 'Untitled Workflow')

// At most three chips are shown in the toolbar; the rest is collapsed
// into a counter so the bar never wraps.
const visibleTags = computed(() => props.workflowTags.slice(0, 3))
const hiddenTagCount = computed(() => Math.max(0, props.workflowTags.length - visibleTags.value.length))

// Mirror the toggle-panel / toggle-AI props into local refs so the
// button highlight updates immediately on click. Without this, the
// class binding `props.showNodePalette ? ...` sometimes stays stale
// on the initial render — Vue's render proxy tracks the dependency
// but the reactive proxy returned by `withDefaults(defineProps<...>())`
// doesn't always trigger re-evaluation when a parent ref toggles
// from one boolean to the same-shape boolean. The `defineProps<...>()`
// return value IS reactive, but binding the conditional directly
// against `props.foo` works fine in some templates and not others
// (this file's Nodes/AI buttons consistently failed while the
// Active/Debug buttons worked — likely due to a Vue compiler quirk
// around optional vs required booleans).
//
// Keeping a local mirror that's explicit-ref + watch makes the
// reactivity unambiguous: every parent prop change flips the local
// ref synchronously, and the class binding reads from the local ref.
const localShowNodePalette = ref(props.showNodePalette)
const localShowAgentAi = ref(props.showAgentAi ?? false)
watch(() => props.showNodePalette, (v) => { localShowNodePalette.value = !!v })
watch(() => props.showAgentAi, (v) => { localShowAgentAi.value = !!v })

watch(
  () => props.workflowName,
  (workflowName) => {
    if (!editingName.value) {
      workflowNameInput.value = workflowName || ''
    }
  },
  { immediate: true }
)

function startEditingName() {
  workflowNameInput.value = props.workflowName || ''
  editingName.value = true
}

function saveWorkflowName() {
  const trimmedName = workflowNameInput.value.trim()
  if (trimmedName) {
    emit('rename', trimmedName)
  }
  editingName.value = false
}

function cancelEditingName() {
  workflowNameInput.value = props.workflowName || ''
  editingName.value = false
}
</script>

<template>
  <!--
    Editor header — n8n parity: a 65px bar with a hairline bottom
    border, a surface background, ghost icon buttons and exactly ONE
    primary action (Save, in brand).
  -->
  <div
    class="m9m-bar h-[65px] shrink-0 flex items-center gap-3 px-3 border-b border-default"
  >
    <!-- Left: back, name, dirty state, tags -->
    <div class="flex items-center gap-3 min-w-0">
      <button
        @click="emit('back')"
        class="h-8 w-8 inline-flex items-center justify-center rounded-md text-neutral-500 dark:text-neutral-400 hover:bg-hover"
        title="Back to workflows"
        aria-label="Back to workflows"
      >
        <ChevronLeftIcon class="w-5 h-5" />
      </button>

      <div v-if="editingName" class="flex items-center gap-2">
        <input
          v-model="workflowNameInput"
          @keyup.enter="saveWorkflowName"
          @keyup.escape="cancelEditingName"
          class="input h-8 w-64 px-2 text-sm font-semibold"
          aria-label="Workflow name"
        />
        <button
          @click="saveWorkflowName"
          class="h-8 px-3 text-sm font-medium rounded-md bg-brand-500 text-white hover:bg-brand-600"
        >
          Save
        </button>
        <button
          @click="cancelEditingName"
          class="h-8 px-3 text-sm font-medium rounded-md text-secondary hover:bg-hover"
        >
          Cancel
        </button>
      </div>
      <div v-else class="flex items-center gap-2 cursor-pointer group min-w-0" @click="startEditingName">
        <h2 class="text-xl font-semibold tracking-tight text-primary truncate">
          {{ displayName }}
        </h2>
        <PencilIcon class="w-4 h-4 text-tertiary opacity-0 group-hover:opacity-100 transition-opacity shrink-0" />
      </div>

      <span
        v-if="props.isDirty"
        class="text-xs text-tertiary shrink-0"
      >
        Unsaved changes
      </span>

      <!-- Tags of the current workflow -->
      <button
        v-if="!props.isNewWorkflow && props.workflowTags.length > 0"
        type="button"
        class="flex items-center gap-1.5 pl-3 border-l border-default shrink-0"
        title="Manage workflow tags"
        @click="emit('showTags')"
      >
        <TagChip
          v-for="tag in visibleTags"
          :key="tag.id"
          :tag="tag"
        />
        <span
          v-if="hiddenTagCount > 0"
          class="text-xs text-tertiary"
        >
          +{{ hiddenTagCount }}
        </span>
      </button>
    </div>

    <!-- Middle: panel toggles -->
    <div class="flex items-center gap-1 ml-auto">
      <button
        @click="emit('togglePalette')"
        :class="[
          'h-8 px-3 text-sm font-medium rounded-md transition-colors',
          localShowNodePalette
            ? 'm9m-toggle-active'
            : 'text-secondary hover:bg-hover'
        ]"
        title="Open the node panel"
      >
        Nodes
      </button>
      <button
        @click="emit('toggleAi')"
        :class="[
          'h-8 px-3 text-sm font-medium rounded-md transition-colors flex items-center gap-1.5',
          localShowAgentAi
            ? 'm9m-toggle-active'
            : 'text-secondary hover:bg-hover'
        ]"
        title="Open the AI assistant"
      >
        <SparklesIcon class="w-4 h-4" />
        AI
      </button>

      <!-- Tag management. Hidden for a draft workflow: it has no id yet,
           so there is nothing to attach the tags to. -->
      <button
        v-if="!props.isNewWorkflow"
        @click="emit('showTags')"
        class="h-8 px-3 text-sm font-medium rounded-md transition-colors flex items-center gap-1.5 text-secondary hover:bg-hover"
        title="Attach tags to this workflow"
      >
        <TagIcon class="w-4 h-4" />
        Tags
      </button>

      <!-- Version history drawer -->
      <button
        v-if="!props.isNewWorkflow"
        @click="emit('showVersions')"
        class="h-8 px-3 text-sm font-medium rounded-md transition-colors flex items-center gap-1.5 text-secondary hover:bg-hover"
        title="Show the version history of this workflow"
      >
        <ClockIcon class="w-4 h-4" />
        Version history
      </button>
    </div>

    <!-- Canvas actions: undo / redo / tidy up / sticky note. -->
    <div class="flex items-center gap-1">
      <button
        @click="emit('undo')"
        :disabled="!props.canUndo"
        class="h-8 w-8 inline-flex items-center justify-center rounded-md text-neutral-500 dark:text-neutral-400 hover:bg-hover disabled:opacity-40 disabled:hover:bg-transparent"
        title="Undo (Ctrl+Z)"
        aria-label="Undo"
      >
        <ArrowUturnLeftIcon class="w-4 h-4" />
      </button>
      <button
        @click="emit('redo')"
        :disabled="!props.canRedo"
        class="h-8 w-8 inline-flex items-center justify-center rounded-md text-neutral-500 dark:text-neutral-400 hover:bg-hover disabled:opacity-40 disabled:hover:bg-transparent"
        title="Redo (Ctrl+Shift+Z)"
        aria-label="Redo"
      >
        <ArrowUturnRightIcon class="w-4 h-4" />
      </button>
      <button
        @click="emit('autoLayout')"
        class="h-8 w-8 inline-flex items-center justify-center rounded-md text-neutral-500 dark:text-neutral-400 hover:bg-hover"
        title="Tidy up — arrange the nodes automatically"
        aria-label="Tidy up"
      >
        <Squares2X2Icon class="w-4 h-4" />
      </button>
      <button
        @click="emit('addStickyNote')"
        class="h-8 w-8 inline-flex items-center justify-center rounded-md text-neutral-500 dark:text-neutral-400 hover:bg-hover"
        title="Add a sticky note"
        aria-label="Add sticky note"
      >
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 4h14v10l-4 4H5V4z" />
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 18v-4h4" />
        </svg>
      </button>
    </div>

    <!-- Right: workflow state + the single primary action. -->
    <div class="flex items-center gap-2">
      <button
        v-if="!props.isNewWorkflow"
        @click="emit('toggleActive')"
        :class="[
          'h-8 px-3 text-sm font-medium rounded-md transition-colors flex items-center gap-2',
          props.workflowActive
            ? 'bg-green-50 text-green-800 dark:bg-green-500/15 dark:text-green-300'
            : 'text-secondary hover:bg-hover'
        ]"
      >
        <component :is="props.workflowActive ? StopIcon : PlayIcon" class="w-4 h-4" />
        {{ props.workflowActive ? 'Active' : 'Inactive' }}
      </button>

      <!-- Debug toggle. Controls whether the engine captures full
           per-node I/O on every run (Debug=ON) or only the start
           + end nodes (Debug=OFF, the production default). When
           ON, the ExecutionDetail view shows full per-node
           debug data; when OFF, only the start/end nodes show
           payload data and intermediate nodes appear as
           "ran, but no per-node snapshot captured". -->
      <button
        v-if="!props.isNewWorkflow"
        @click="emit('toggleDebug')"
        :class="[
          'h-8 px-3 text-sm font-medium rounded-md transition-colors flex items-center gap-2',
          props.workflowDebug
            ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-500/15 dark:text-yellow-300'
            : 'text-secondary hover:bg-hover'
        ]"
        :title="props.workflowDebug
          ? 'Debug ON — execution detail shows full per-node input/output'
          : 'Debug OFF — execution detail only captures start and end nodes'"
      >
        <BugAntIcon class="w-4 h-4" />
        {{ props.workflowDebug ? 'Debug ON' : 'Debug OFF' }}
      </button>

      <button
        @click="emit('execute')"
        :disabled="props.isExecuting || props.isNewWorkflow"
        class="h-8 px-3 text-sm font-medium rounded-md flex items-center gap-2 border border-strong bg-surface text-primary hover:bg-hover disabled:opacity-50 disabled:cursor-not-allowed"
      >
        <ArrowPathIcon v-if="props.isExecuting" class="w-4 h-4 animate-spin" />
        <PlayIcon v-else class="w-4 h-4" />
        <span>{{ props.isExecuting ? 'Running...' : 'Execute' }}</span>
      </button>

      <!-- Download the workflow as an n8n-compatible JSON file.
           Hidden for a draft workflow: it has no id yet, so the
           export endpoint has nothing to serve. -->
      <button
        v-if="!props.isNewWorkflow"
        @click="emit('export')"
        :disabled="props.isExporting"
        class="h-8 px-3 text-sm font-medium rounded-md flex items-center gap-2 border border-strong bg-surface text-primary hover:bg-hover disabled:opacity-50"
        title="Download this workflow as an n8n-compatible JSON file"
      >
        <ArrowDownTrayIcon class="w-4 h-4" />
        <span>{{ props.isExporting ? 'Exporting...' : 'Export' }}</span>
      </button>

      <!-- The one primary action of the editor. -->
      <button
        @click="emit('save')"
        :disabled="props.isLoading"
        class="m9m-btn-primary h-8 px-3 text-sm font-medium rounded-md flex items-center gap-2 text-white disabled:opacity-50 disabled:cursor-not-allowed"
      >
        <CloudArrowUpIcon class="w-4 h-4" />
        <span>{{ props.isLoading ? 'Saving...' : 'Save' }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
/*
 * The token layer publishes `--background--surface` and
 * `--background--brand`; the fallbacks keep the bar and the primary
 * button correct until (or if) it lands.
 */
.m9m-bar {
  background: var(--background--surface, #ffffff);
}

:global(.dark) .m9m-bar {
  background: var(--background--surface, oklch(26.86% 0 89.88));
}

.m9m-btn-primary {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.m9m-btn-primary:hover:not(:disabled) {
  background: var(--background--brand--hover, oklch(68.08% 0.148 5.02));
}

.m9m-btn-primary:active:not(:disabled) {
  background: var(--background--brand--active, oklch(59.09% 0.1611 5.09));
}

/*
 * The active state of the Nodes / AI toggles. The brand ramp is
 * published by the foundation agent as `brand-50` / `brand-700`;
 * the CSS-variable fallback keeps the highlight visible before it
 * lands.
 */
.m9m-toggle-active {
  background: var(--background--brand--tint, oklch(97.02% 0 89.88));
  color: var(--background--brand--tint--foreground, oklch(38.67% 0 89.88));
}

:global(.dark) .m9m-toggle-active {
  background: var(--background--brand--tint--dark, oklch(31.71% 0.136 5.02 / 0.15));
  color: var(--background--brand--tint--foreground--dark, oklch(85.55% 0.105 5.02));
}

@media (prefers-reduced-motion: reduce) {
  .m9m-btn-primary {
    transition-duration: 1ms;
  }
}
</style>
