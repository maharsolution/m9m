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
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'

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
    class="fixed inset-0 z-50 flex items-center justify-center bg-[oklch(100%_0_89.88/0.8)] p-4 backdrop-blur-[2px] dark:bg-black/60"
    @click.self="emit('close')"
  >
    <div class="flex max-h-[85vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <div class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-950/50 dark:text-brand-400">
            <TagIcon class="h-4 w-4" />
          </div>
          <div>
            <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
              Workflow tags
            </h2>
            <p class="text-xs text-neutral-600 dark:text-neutral-400">
              Attach tags so this workflow can be found and filtered.
            </p>
          </div>
        </div>
        <IconButton
          label="Close"
          @click="emit('close')"
        >
          <XMarkIcon class="h-4 w-4" />
        </IconButton>
      </div>

      <!-- Body -->
      <div class="flex-1 space-y-5 overflow-y-auto p-6">
        <!-- Notice -->
        <div
          v-if="notice"
          class="flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
          :class="notice.type === 'success'
            ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/50 dark:text-green-300'
            : 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300'"
        >
          <ExclamationTriangleIcon
            v-if="notice.type === 'error'"
            class="mt-0.5 h-4 w-4 shrink-0"
          />
          <p class="flex-1">{{ notice.message }}</p>
          <button
            type="button"
            class="rounded-sm opacity-60 transition-opacity hover:opacity-100"
            aria-label="Dismiss"
            @click="notice = null"
          >
            <XMarkIcon class="h-4 w-4" />
          </button>
        </div>

        <!-- Current tags -->
        <div>
          <h3 class="mb-2 text-sm font-semibold text-neutral-950 dark:text-neutral-50">
            Attached tags
          </h3>
          <div
            v-if="loading"
            class="text-sm text-neutral-500 dark:text-neutral-400"
          >
            Loading tags…
          </div>
          <div
            v-else-if="assignedTags.length === 0"
            class="rounded-lg border border-dashed border-neutral-300 px-4 py-6 text-center text-sm text-neutral-500 dark:border-neutral-700 dark:text-neutral-400"
          >
            This workflow has no tags yet.
          </div>
          <div
            v-else
            class="flex flex-wrap gap-2"
          >
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
          <div class="mb-2 flex items-center justify-between">
            <h3 class="text-sm font-semibold text-neutral-950 dark:text-neutral-50">
              All tags
            </h3>
            <Button
              variant="ghost"
              size="sm"
              @click="showCreate = !showCreate"
            >
              <template #icon>
                <PlusIcon class="h-3.5 w-3.5" />
              </template>
              <span>New tag</span>
            </Button>
          </div>

          <!-- Inline create -->
          <div
            v-if="showCreate"
            class="mb-3 space-y-3 rounded-lg border border-neutral-200 p-3 dark:border-neutral-800"
          >
            <div class="flex flex-col gap-2 sm:flex-row">
              <input
                v-model="newName"
                type="text"
                placeholder="Tag name"
                class="input flex-1"
                @keyup.enter="createAndAttach"
              >
              <Button
                variant="primary"
                size="sm"
                :disabled="creating"
                :loading="creating"
                @click="createAndAttach"
              >
                <template #icon>
                  <PlusIcon class="h-4 w-4" />
                </template>
                <span>Create &amp; attach</span>
              </Button>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button
                v-for="color in TAG_PALETTE"
                :key="color"
                type="button"
                class="h-5 w-5 rounded-full border-2 transition-transform hover:scale-110"
                :class="newColor === color ? 'border-neutral-900 dark:border-white' : 'border-transparent'"
                :style="{ backgroundColor: color }"
                :aria-label="`Use colour ${color}`"
                :title="color"
                @click="newColor = color"
              />
            </div>
          </div>

          <!-- Loading -->
          <div
            v-if="loading"
            class="py-6 text-center"
          >
            <span
              class="mx-auto inline-block h-6 w-6 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
              aria-hidden="true"
            />
          </div>

          <!-- Empty -->
          <div
            v-else-if="allTags.length === 0"
            class="rounded-lg border border-dashed border-neutral-300 px-4 py-6 text-center text-sm text-neutral-500 dark:border-neutral-700 dark:text-neutral-400"
          >
            No tags exist yet — create one above.
          </div>

          <!-- Rows -->
          <ul
            v-else
            class="divide-y divide-neutral-100 rounded-lg border border-neutral-200 dark:divide-neutral-800 dark:border-neutral-800"
          >
            <li
              v-for="tag in allTags"
              :key="tag.id"
              class="flex items-center gap-3 px-3 py-2"
            >
              <TagChip :tag="tag" />
              <span class="flex-1" />
              <Button
                v-if="assignedIds.has(tag.id)"
                variant="ghost"
                size="sm"
                class="text-red-700 dark:text-red-400"
                :disabled="busyTagId === tag.id"
                @click="detach(tag)"
              >
                <template #icon>
                  <XMarkIcon class="h-3.5 w-3.5" />
                </template>
                <span>Detach</span>
              </Button>
              <Button
                v-else
                variant="secondary"
                size="sm"
                :disabled="busyTagId === tag.id"
                @click="attach(tag)"
              >
                <template #icon>
                  <CheckIcon class="h-3.5 w-3.5" />
                </template>
                <span>Attach</span>
              </Button>
            </li>
          </ul>
        </div>
      </div>

      <!-- Footer -->
      <div class="flex justify-end border-t border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <Button
          variant="secondary"
          size="sm"
          @click="emit('close')"
        >
          Done
        </Button>
      </div>
    </div>
  </div>
</template>
