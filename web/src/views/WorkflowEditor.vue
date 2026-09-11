<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
import WorkflowCanvas from '@/components/workflow/WorkflowCanvas.vue'
import WorkflowEditorToolbar from '@/components/workflow/WorkflowEditorToolbar.vue'
import NodePalette from '@/components/workflow/NodePalette.vue'
import NodePanel from '@/components/workflow/NodePanel.vue'
import AgentAI from '@/components/ai/AgentAI.vue'
import { useWorkflowEditorStore, useWorkflowStore } from '@/stores'

const route = useRoute()
const router = useRouter()
const workflowStore = useWorkflowStore()
const workflowEditorStore = useWorkflowEditorStore()

const showNodePalette = ref(true)
const showNodePanel = ref(false)
const showAI = ref(false)
const isExecuting = ref(false)

const workflowId = computed(() => route.params.id as string | undefined)
const isNewWorkflow = computed(() => !workflowId.value || workflowId.value === 'new')
const workflow = computed(() => workflowStore.currentWorkflow)
const selectedNode = computed(() => workflowEditorStore.selectedNode)

onMounted(async () => {
  if (!isNewWorkflow.value && workflowId.value) {
    await workflowStore.fetchWorkflow(workflowId.value)
    workflowEditorStore.resetEditorState()
  } else {
    workflowEditorStore.createNewWorkflow()
  }
})

// Re-initialise the editor when the route changes between two
// existing workflows OR between an existing workflow and
// `/workflows/new`. Without this watch, navigating from a
// previously-saved workflow to a fresh one keeps the previous
// nodes on the canvas (because Vue Router reuses the same
// component instance, so onMounted never fires again) — which
// is the user-reported "New Workflow still shows the previous
// nodes" bug. The watcher mirrors the onMounted branch exactly
// so reload-on-route and initial-mount behave identically.
//
// Skips:
//   * When the new id already matches the workflow currently
//     loaded in the store. That's the post-save path:
//     saveWorkflow() routes from /workflows/new → /workflows/{id}
//     and the store already holds the freshly-saved workflow,
//     so re-fetching would just be an extra round-trip for
//     no gain.
//   * When arriving at /workflows/new with no real id in the
//     store (e.g. the Duplicate action in WorkflowList seeds
//     a fresh draft with id='' before navigating). Wiping the
//     draft would discard the duplicated content.
watch(
  workflowId,
  async (newId, oldId) => {
    if (newId === oldId) return
    if (!newId || newId === 'new') {
      const current = workflow.value
      // `current.id === ''` is the fresh-draft signal: the store
      // holds a workflow someone just placed but it hasn't been
      // persisted yet (Duplicate action, New button via onMounted,
      // etc.). `!current.id` covers the onMounted-direct case where
      // currentWorkflow was never set, and the store falls back to
      // null when no draft was prepared.
      if (current && current.id === '') {
        return
      }
      workflowEditorStore.createNewWorkflow()
      return
    }
    if (workflow.value?.id === newId) return
    try {
      await workflowStore.fetchWorkflow(newId)
      workflowEditorStore.resetEditorState()
    } catch (e) {
      console.error('Failed to load workflow on route change:', e)
    }
  }
)

// Watch for selected node changes to show/hide panel
watch(selectedNode, (node) => {
  showNodePanel.value = !!node
})

// Confirm before leaving with unsaved changes
onBeforeRouteLeave((_to, _from, next) => {
  if (workflowEditorStore.isDirty) {
    if (confirm('You have unsaved changes. Are you sure you want to leave?')) {
      next()
    } else {
      next(false)
    }
  } else {
    next()
  }
})

const saveWorkflow = async () => {
  try {
    await workflowStore.saveWorkflow()
    workflowEditorStore.markClean()
    if (isNewWorkflow.value && workflow.value?.id) {
      router.replace(`/workflows/${workflow.value.id}`)
    }
  } catch (e) {
    console.error('Failed to save workflow:', e)
    alert('Failed to save workflow. Please try again.')
  }
}

const executeWorkflow = async () => {
  if (!workflow.value?.id) {
    alert('Please save the workflow before executing')
    return
  }

  isExecuting.value = true
  try {
    const execution = await workflowStore.executeWorkflow(workflow.value.id)
    // Could show execution result or navigate to execution detail
    console.log('Execution started:', execution)
  } catch (e) {
    console.error('Failed to execute workflow:', e)
    alert('Failed to execute workflow. Please try again.')
  } finally {
    isExecuting.value = false
  }
}

const toggleActive = async () => {
  if (!workflow.value?.id) return
  await workflowStore.toggleWorkflowActive(workflow.value.id)
}

// toggleDebug flips workflow.Debug locally + immediately persists
// it through the regular save path. The button in the toolbar
// stays in sync because the toolbar reads `workflow?.debug`
// directly from the store, so a save → refetch is unnecessary.
const toggleDebug = async () => {
  if (!workflow.value) return
  // Mutate the workflow object in-place so Vue's reactivity picks
  // it up. We don't need to await anything before the visual
  // state changes — the toolbar re-reads on the next tick.
  workflow.value.debug = !workflow.value.debug
  workflowEditorStore.markDirty()
  // Persist straight away so a navigation away from the editor
  // doesn't lose the toggle. If this fails the user can still
  // hit the Save button to retry.
  try {
    await workflowStore.saveWorkflow()
    workflowEditorStore.markClean()
  } catch (e) {
    console.error('Failed to persist debug flag:', e)
  }
}

const renameWorkflow = (name: string) => {
  workflowEditorStore.setWorkflowName(name)
}
</script>

<template>
  <div class="h-full flex flex-col bg-canvas-light dark:bg-canvas-dark">
    <WorkflowEditorToolbar
      :workflow-name="workflow?.name"
      :workflow-active="workflow?.active"
      :workflow-debug="workflow?.debug"
      :is-new-workflow="isNewWorkflow"
      :is-dirty="workflowEditorStore.isDirty"
      :is-loading="workflowStore.loading"
      :is-executing="isExecuting"
      :show-node-palette="showNodePalette"
      :show-agent-ai="showAI"
      @back="router.push('/workflows')"
      @rename="renameWorkflow"
      @toggle-palette="showNodePalette = !showNodePalette"
      @toggle-ai="showAI = !showAI"
      @toggle-active="toggleActive"
      @toggle-debug="toggleDebug"
      @execute="executeWorkflow"
      @save="saveWorkflow"
    />

    <div class="flex-1 flex overflow-hidden">
      <transition
        enter-active-class="transition-all duration-200 ease-out"
        enter-from-class="-translate-x-full opacity-0"
        enter-to-class="translate-x-0 opacity-100"
        leave-active-class="transition-all duration-150 ease-in"
        leave-from-class="translate-x-0 opacity-100"
        leave-to-class="-translate-x-full opacity-0"
      >
        <NodePalette v-if="showNodePalette" class="w-72" />
      </transition>

      <div class="flex-1 relative">
        <WorkflowCanvas />
      </div>

      <transition
        enter-active-class="transition-all duration-200 ease-out"
        enter-from-class="translate-x-full opacity-0"
        enter-to-class="translate-x-0 opacity-100"
        leave-active-class="transition-all duration-150 ease-in"
        leave-from-class="translate-x-0 opacity-100"
        leave-to-class="translate-x-full opacity-0"
      >
        <NodePanel v-if="showNodePanel && selectedNode" class="w-80" />
      </transition>

      <transition
        enter-active-class="transition-all duration-200 ease-out"
        enter-from-class="translate-x-full opacity-0"
        enter-to-class="translate-x-0 opacity-100"
        leave-active-class="transition-all duration-150 ease-in"
        leave-from-class="translate-x-0 opacity-100"
        leave-to-class="translate-x-full opacity-0"
      >
        <AgentAI
          v-if="showAI"
          class="w-96"
          @close="showAI = false"
        />
      </transition>
    </div>
  </div>
</template>
