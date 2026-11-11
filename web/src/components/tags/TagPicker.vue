<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import {
  XMarkIcon,
  TagIcon,
  PlusIcon,
  CheckIcon,
  ExclamationTriangleIcon,
} from '@heroicons/vue/24/outline'
import { useTagsStore } from '@/stores/tags'
import { tagErrorMessage } from '@/api/tags'
import type { Tag } from '@/types/tag'
import { TAG_PALETTE, DEFAULT_TAG_COLOR, normalizeHexColor } from '@/types/tag'
import TagChip from './TagChip.vue'

const props = defineProps<{
  workflowId: string
}>()

const emit = defineEmits<{
  close: []
  updated: [tags: Tag[]]
}>()

const tagsStore = useTagsStore()

const busyTagId = ref<string | null>(null)
const loading = ref(true)
const notice = ref<{ type: 'success' | 'error'; message: string } | null>(null)

const showCreate = ref(false)
const newName = ref('')
const newColor = ref<string>(DEFAULT_TAG_COLOR)
const creating = ref(false)

const allTags = computed(() => tagsStore.sortedTags)

const assignedTags = computed<Tag[]>(() => tagsStore.getWorkflowTagList(props.workflowId))

const assignedIds = computed(() => new Set(assignedTags.value.map((tag) => tag.id)))

onMounted(async () => {
  loading.value = true
  try {
    await Promise.all([
      tagsStore.fetchAllTags(),
      tagsStore.fetchWorkflowTags(props.workflowId),
    ])
  } catch {
    // The store holds the error message; it is rendered below.
  } finally {
    loading.value = false
  }
})

async function attach(tag: Tag) {
  busyTagId.value = tag.id
  notice.value = null
  try {
    const updated = await tagsStore.addWorkflowTag(props.workflowId, tag.id)
    notice.value = { type: 'success', message: `Tag "${tag.name}" attached.` }
    emit('updated', updated)
  } catch (e) {
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to attach the tag.'),
    }
  } finally {
    busyTagId.value = null
  }
}

async function detach(tag: Tag) {
  busyTagId.value = tag.id
  notice.value = null
  try {
    await tagsStore.removeWorkflowTag(props.workflowId, tag.id)
    notice.value = { type: 'success', message: `Tag "${tag.name}" detached.` }
    emit('updated', tagsStore.getWorkflowTagList(props.workflowId))
  } catch (e) {
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to detach the tag.'),
    }
  } finally {
    busyTagId.value = null
  }
}

async function createAndAttach() {
  const name = newName.value.trim()
  if (!name) {
    notice.value = { type: 'error', message: 'A tag needs a name.' }
    return
  }

  creating.value = true
  notice.value = null
  try {
    const tag = await tagsStore.createTag({
      name,
      color: normalizeHexColor(newColor.value) ?? DEFAULT_TAG_COLOR,
    })
    const updated = await tagsStore.addWorkflowTag(props.workflowId, tag.id)
    newName.value = ''
    newColor.value = DEFAULT_TAG_COLOR
    showCreate.value = false
    notice.value = { type: 'success', message: `Tag "${tag.name}" created and attached.` }
    emit('updated', updated)
  } catch (e) {
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to create the tag.'),
    }
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div
    class="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4"
    @click.self="emit('close')"
  >
    <div class="card w-full max-w-xl max-h-[85vh] flex flex-col overflow-hidden">
      <!-- Header -->
      <div class="flex items-center justify-between px-6 py-4 border-b border-slate-200 dark:border-slate-700">
        <div class="flex items-center gap-3">
          <div class="p-2 bg-primary-100 dark:bg-primary-900/30 rounded-lg">
            <TagIcon class="w-5 h-5 text-primary-600 dark:text-primary-400" />
          </div>
          <div>
            <h2 class="text-lg font-semibold text-slate-900 dark:text-white">Workflow tags</h2>
            <p class="text-xs text-slate-500 dark:text-slate-400">
              Attach tags so this workflow can be found and filtered.
            </p>
          </div>
        </div>
        <button
          type="button"
          class="p-2 text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-700"
          title="Close"
          aria-label="Close"
          @click="emit('close')"
        >
          <XMarkIcon class="w-5 h-5" />
        </button>
      </div>

      <!-- Body -->
      <div class="flex-1 overflow-y-auto p-6 space-y-5">
        <!-- Notice -->
        <div
          v-if="notice"
          :class="notice.type === 'success'
            ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-300'
            : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-800 dark:text-red-300'"
          class="flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
        >
          <ExclamationTriangleIcon v-if="notice.type === 'error'" class="w-5 h-5 shrink-0 mt-0.5" />
          <p class="flex-1">{{ notice.message }}</p>
          <button type="button" class="opacity-60 hover:opacity-100" @click="notice = null">
            <XMarkIcon class="w-4 h-4" />
          </button>
        </div>

        <!-- Current tags -->
        <div>
          <h3 class="text-sm font-semibold text-slate-900 dark:text-white mb-2">
            Attached tags
          </h3>
          <div v-if="loading" class="text-sm text-slate-500 dark:text-slate-400">
            Loading tags...
          </div>
          <div
            v-else-if="assignedTags.length === 0"
            class="rounded-lg border border-dashed border-slate-300 dark:border-slate-600 px-4 py-6 text-center text-sm text-slate-500 dark:text-slate-400"
          >
            This workflow has no tags yet.
          </div>
          <div v-else class="flex flex-wrap gap-2">
            <TagChip
              v-for="tag in assignedTags"
              :key="tag.id"
              :tag="tag"
              size="md"
              removable
              @remove="detach(tag)"
            />
          </div>
        </div>

        <!-- All tags -->
        <div>
          <div class="flex items-center justify-between mb-2">
            <h3 class="text-sm font-semibold text-slate-900 dark:text-white">
              All tags
            </h3>
            <button
              type="button"
              class="btn-ghost py-1 px-2 text-xs flex items-center gap-1"
              @click="showCreate = !showCreate"
            >
              <PlusIcon class="w-3.5 h-3.5" />
              New tag
            </button>
          </div>

          <!-- Inline create -->
          <div
            v-if="showCreate"
            class="rounded-lg border border-slate-200 dark:border-slate-700 p-3 mb-3 space-y-3"
          >
            <div class="flex flex-col sm:flex-row gap-2">
              <input
                v-model="newName"
                type="text"
                placeholder="Tag name"
                class="input flex-1"
                @keyup.enter="createAndAttach"
              />
              <button
                type="button"
                class="btn-primary flex items-center justify-center gap-2"
                :disabled="creating"
                @click="createAndAttach"
              >
                <PlusIcon class="w-4 h-4" />
                Create &amp; attach
              </button>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button
                v-for="color in TAG_PALETTE"
                :key="color"
                type="button"
                class="w-5 h-5 rounded-full border-2 transition-transform hover:scale-110"
                :class="newColor === color ? 'border-slate-900 dark:border-white' : 'border-transparent'"
                :style="{ backgroundColor: color }"
                :aria-label="`Use colour ${color}`"
                :title="color"
                @click="newColor = color"
              />
            </div>
          </div>

          <!-- Loading -->
          <div v-if="loading" class="py-6 text-center">
            <div class="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600 mx-auto" />
          </div>

          <!-- Empty -->
          <div
            v-else-if="allTags.length === 0"
            class="rounded-lg border border-dashed border-slate-300 dark:border-slate-600 px-4 py-6 text-center text-sm text-slate-500 dark:text-slate-400"
          >
            No tags exist yet — create one above.
          </div>

          <!-- Rows -->
          <ul
            v-else
            class="divide-y divide-slate-100 dark:divide-slate-700 rounded-lg border border-slate-200 dark:border-slate-700"
          >
            <li v-for="tag in allTags" :key="tag.id" class="px-3 py-2 flex items-center gap-3">
              <TagChip :tag="tag" />
              <span class="flex-1" />
              <button
                v-if="assignedIds.has(tag.id)"
                type="button"
                class="btn-ghost py-1 px-2 text-xs flex items-center gap-1 text-red-600 dark:text-red-400"
                :disabled="busyTagId === tag.id"
                @click="detach(tag)"
              >
                <XMarkIcon class="w-3.5 h-3.5" />
                Detach
              </button>
              <button
                v-else
                type="button"
                class="btn-secondary py-1 px-2 text-xs flex items-center gap-1"
                :disabled="busyTagId === tag.id"
                @click="attach(tag)"
              >
                <CheckIcon class="w-3.5 h-3.5" />
                Attach
              </button>
            </li>
          </ul>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t border-slate-200 dark:border-slate-700 flex justify-end">
        <button type="button" class="btn-secondary" @click="emit('close')">Done</button>
      </div>
    </div>
  </div>
</template>
