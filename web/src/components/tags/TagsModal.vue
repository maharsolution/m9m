<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import {
  XMarkIcon,
  TagIcon,
  PlusIcon,
  PencilSquareIcon,
  TrashIcon,
  CheckIcon,
} from '@heroicons/vue/24/outline'
import { useTagsStore } from '@/stores/tags'
import { tagErrorMessage } from '@/api/tags'
import type { Tag } from '@/types/tag'
import { TAG_PALETTE, DEFAULT_TAG_COLOR, normalizeHexColor } from '@/types/tag'
import TagChip from './TagChip.vue'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'

const emit = defineEmits<{
  close: []
}>()

const tagsStore = useTagsStore()

const newName = ref('')
const newColor = ref<string>(DEFAULT_TAG_COLOR)
const creating = ref(false)

const editingId = ref<string | null>(null)
const editName = ref('')
const editColor = ref<string>(DEFAULT_TAG_COLOR)
const saving = ref(false)

const notice = ref<{ type: 'success' | 'error'; message: string } | null>(null)

const tags = computed(() => tagsStore.sortedTags)

onMounted(async () => {
  try {
    await tagsStore.fetchAllTags()
  } catch {
    // The store already holds the error message.
  }
})

function pickColor(target: 'new' | 'edit', color: string) {
  if (target === 'new') {
    newColor.value = color
  } else {
    editColor.value = color
  }
}

async function handleCreate() {
  const name = newName.value.trim()
  if (!name) {
    notice.value = { type: 'error', message: 'A tag needs a name.' }
    return
  }

  creating.value = true
  notice.value = null
  try {
    await tagsStore.createTag({
      name,
      color: normalizeHexColor(newColor.value) ?? DEFAULT_TAG_COLOR,
    })
    newName.value = ''
    newColor.value = DEFAULT_TAG_COLOR
    notice.value = { type: 'success', message: `Tag "${name}" created.` }
  } catch (e) {
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to create the tag.'),
    }
  } finally {
    creating.value = false
  }
}

function startEdit(tag: Tag) {
  editingId.value = tag.id
  editName.value = tag.name
  editColor.value = normalizeHexColor(tag.color) ?? DEFAULT_TAG_COLOR
  notice.value = null
}

function cancelEdit() {
  editingId.value = null
  editName.value = ''
  editColor.value = DEFAULT_TAG_COLOR
}

async function handleSave(tag: Tag) {
  const name = editName.value.trim()
  if (!name) {
    notice.value = { type: 'error', message: 'A tag needs a name.' }
    return
  }

  saving.value = true
  notice.value = null
  try {
    await tagsStore.updateTag(tag.id, {
      name,
      color: normalizeHexColor(editColor.value) ?? DEFAULT_TAG_COLOR,
    })
    notice.value = { type: 'success', message: `Tag "${name}" updated.` }
    cancelEdit()
  } catch (e) {
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to update the tag.'),
    }
  } finally {
    saving.value = false
  }
}

async function handleDelete(tag: Tag) {
  const confirmed = confirm(
    `Delete the tag "${tag.name}"?\n\nThe tag is removed from every workflow that uses it. This cannot be undone.`
  )
  if (!confirmed) return

  notice.value = null
  try {
    await tagsStore.deleteTag(tag.id)
    notice.value = { type: 'success', message: `Tag "${tag.name}" deleted.` }
    if (editingId.value === tag.id) cancelEdit()
  } catch (e) {
    // The backend refuses the delete while the tag is still attached
    // to a workflow — surface that message verbatim.
    notice.value = {
      type: 'error',
      message: tagErrorMessage(e, 'Failed to delete the tag.'),
    }
  }
}
</script>

<template>
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-[oklch(100%_0_89.88/0.8)] p-4 backdrop-blur-[2px] dark:bg-black/60"
    @click.self="emit('close')"
  >
    <div class="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <div class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-950/50 dark:text-brand-400">
            <TagIcon class="h-4 w-4" />
          </div>
          <div>
            <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
              Manage tags
            </h2>
            <p class="text-xs text-neutral-600 dark:text-neutral-400">
              Tags group workflows so they can be found and filtered quickly.
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
      <div class="flex-1 space-y-6 overflow-y-auto p-6">
        <!-- Notice -->
        <div
          v-if="notice"
          class="flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
          :class="notice.type === 'success'
            ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/50 dark:text-green-300'
            : 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300'"
        >
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

        <!-- Create -->
        <div class="space-y-3 rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
          <h3 class="text-sm font-semibold text-neutral-950 dark:text-neutral-50">
            New tag
          </h3>
          <div class="flex flex-col gap-3 sm:flex-row">
            <input
              v-model="newName"
              type="text"
              placeholder="Tag name (e.g. production)"
              class="input flex-1"
              @keyup.enter="handleCreate"
            >
            <Button
              variant="primary"
              size="sm"
              :disabled="creating"
              :loading="creating"
              @click="handleCreate"
            >
              <template #icon>
                <PlusIcon class="h-4 w-4" />
              </template>
              <span>{{ creating ? 'Creating…' : 'Create' }}</span>
            </Button>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button
              v-for="color in TAG_PALETTE"
              :key="color"
              type="button"
              class="h-6 w-6 rounded-full border-2 transition-transform hover:scale-110"
              :class="newColor === color ? 'border-neutral-900 dark:border-white' : 'border-transparent'"
              :style="{ backgroundColor: color }"
              :aria-label="`Use colour ${color}`"
              :title="color"
              @click="pickColor('new', color)"
            />
            <label class="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
              Custom
              <input
                v-model="newColor"
                type="color"
                class="h-8 w-8 cursor-pointer rounded border border-neutral-300 bg-transparent dark:border-neutral-700"
                aria-label="Custom colour"
              >
            </label>
          </div>
        </div>

        <!-- List -->
        <div>
          <h3 class="mb-3 text-sm font-semibold text-neutral-950 dark:text-neutral-50">
            Existing tags
            <span class="text-xs font-normal text-neutral-500 dark:text-neutral-400">
              ({{ tags.length }})
            </span>
          </h3>

          <!-- Loading -->
          <div
            v-if="tagsStore.loading && tags.length === 0"
            class="py-8 text-center"
          >
            <span
              class="mx-auto inline-block h-8 w-8 animate-spin rounded-full border-2 border-neutral-300 border-t-brand-500 dark:border-neutral-700"
              aria-hidden="true"
            />
            <p class="mt-3 text-sm text-neutral-600 dark:text-neutral-400">
              Loading tags…
            </p>
          </div>

          <!-- Empty -->
          <div
            v-else-if="tags.length === 0"
            class="rounded-lg border border-dashed border-neutral-300 py-10 text-center dark:border-neutral-700"
          >
            <TagIcon class="mx-auto h-10 w-10 text-neutral-300 dark:text-neutral-700" />
            <p class="mt-2 text-sm text-neutral-600 dark:text-neutral-400">
              No tags yet
            </p>
            <p class="text-xs text-neutral-500 dark:text-neutral-500">
              Create the first one above.
            </p>
          </div>

          <!-- Rows -->
          <ul
            v-else
            class="divide-y divide-neutral-100 rounded-lg border border-neutral-200 dark:divide-neutral-800 dark:border-neutral-800"
          >
            <li
              v-for="tag in tags"
              :key="tag.id"
              class="flex items-center gap-3 px-4 py-3"
            >
              <!-- Edit mode -->
              <template v-if="editingId === tag.id">
                <input
                  v-model="editName"
                  type="text"
                  class="input flex-1"
                  @keyup.enter="handleSave(tag)"
                  @keyup.esc="cancelEdit"
                >
                <div class="flex flex-wrap items-center gap-1">
                  <button
                    v-for="color in TAG_PALETTE"
                    :key="color"
                    type="button"
                    class="h-5 w-5 rounded-full border-2 transition-transform hover:scale-110"
                    :class="editColor === color ? 'border-neutral-900 dark:border-white' : 'border-transparent'"
                    :style="{ backgroundColor: color }"
                    :aria-label="`Use colour ${color}`"
                    :title="color"
                    @click="pickColor('edit', color)"
                  />
                </div>
                <button
                  type="button"
                  class="flex h-8 w-8 items-center justify-center rounded-lg bg-green-50 text-green-700 transition-colors hover:bg-green-100 dark:bg-green-950/50 dark:text-green-300"
                  title="Save"
                  aria-label="Save tag"
                  :disabled="saving"
                  @click="handleSave(tag)"
                >
                  <CheckIcon class="h-4 w-4" />
                </button>
                <IconButton
              label="Cancel edit"
              @click="cancelEdit"
            >
              <XMarkIcon class="h-4 w-4" />
            </IconButton>
              </template>

              <!-- View mode -->
              <template v-else>
                <TagChip :tag="tag" />
                <span class="flex-1" />
                <IconButton
                  label="Rename or recolour"
                  @click="startEdit(tag)"
                >
                  <PencilSquareIcon class="h-4 w-4" />
                </IconButton>
                <IconButton
                  label="Delete tag"
                  tone="danger"
                  @click="handleDelete(tag)"
                >
                  <TrashIcon class="h-4 w-4" />
                </IconButton>
              </template>
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
          Close
        </Button>
      </div>
    </div>
  </div>
</template>
