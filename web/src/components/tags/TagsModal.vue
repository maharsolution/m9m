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
    class="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4"
    @click.self="emit('close')"
  >
    <div class="card w-full max-w-2xl max-h-[85vh] flex flex-col overflow-hidden">
      <!-- Header -->
      <div class="flex items-center justify-between px-6 py-4 border-b border-slate-200 dark:border-slate-700">
        <div class="flex items-center gap-3">
          <div class="p-2 bg-primary-100 dark:bg-primary-900/30 rounded-lg">
            <TagIcon class="w-5 h-5 text-primary-600 dark:text-primary-400" />
          </div>
          <div>
            <h2 class="text-lg font-semibold text-slate-900 dark:text-white">Manage tags</h2>
            <p class="text-xs text-slate-500 dark:text-slate-400">
              Tags group workflows so they can be found and filtered quickly.
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
      <div class="flex-1 overflow-y-auto p-6 space-y-6">
        <!-- Notice -->
        <div
          v-if="notice"
          :class="notice.type === 'success'
            ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800 text-green-800 dark:text-green-300'
            : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800 text-red-800 dark:text-red-300'"
          class="flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
        >
          <p class="flex-1">{{ notice.message }}</p>
          <button type="button" class="opacity-60 hover:opacity-100" @click="notice = null">
            <XMarkIcon class="w-4 h-4" />
          </button>
        </div>

        <!-- Create -->
        <div class="rounded-lg border border-slate-200 dark:border-slate-700 p-4 space-y-3">
          <h3 class="text-sm font-semibold text-slate-900 dark:text-white">New tag</h3>
          <div class="flex flex-col sm:flex-row gap-3">
            <input
              v-model="newName"
              type="text"
              placeholder="Tag name (e.g. production)"
              class="input flex-1"
              @keyup.enter="handleCreate"
            />
            <button
              type="button"
              class="btn-primary flex items-center justify-center gap-2"
              :disabled="creating"
              @click="handleCreate"
            >
              <PlusIcon class="w-4 h-4" />
              {{ creating ? 'Creating...' : 'Create' }}
            </button>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <button
              v-for="color in TAG_PALETTE"
              :key="color"
              type="button"
              class="w-6 h-6 rounded-full border-2 transition-transform hover:scale-110"
              :class="newColor === color ? 'border-slate-900 dark:border-white' : 'border-transparent'"
              :style="{ backgroundColor: color }"
              :aria-label="`Use colour ${color}`"
              :title="color"
              @click="pickColor('new', color)"
            />
            <label class="flex items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
              Custom
              <input
                v-model="newColor"
                type="color"
                class="w-8 h-8 rounded border border-slate-300 dark:border-slate-600 bg-transparent cursor-pointer"
                aria-label="Custom colour"
              />
            </label>
          </div>
        </div>

        <!-- List -->
        <div>
          <h3 class="text-sm font-semibold text-slate-900 dark:text-white mb-3">
            Existing tags
            <span class="text-xs font-normal text-slate-500 dark:text-slate-400">
              ({{ tags.length }})
            </span>
          </h3>

          <!-- Loading -->
          <div v-if="tagsStore.loading && tags.length === 0" class="py-8 text-center">
            <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600 mx-auto" />
            <p class="mt-3 text-sm text-slate-500 dark:text-slate-400">Loading tags...</p>
          </div>

          <!-- Empty -->
          <div
            v-else-if="tags.length === 0"
            class="rounded-lg border border-dashed border-slate-300 dark:border-slate-600 py-10 text-center"
          >
            <TagIcon class="w-10 h-10 mx-auto text-slate-300 dark:text-slate-600" />
            <p class="mt-2 text-sm text-slate-600 dark:text-slate-400">No tags yet</p>
            <p class="text-xs text-slate-500 dark:text-slate-500">
              Create the first one above.
            </p>
          </div>

          <!-- Rows -->
          <ul v-else class="divide-y divide-slate-100 dark:divide-slate-700 rounded-lg border border-slate-200 dark:border-slate-700">
            <li
              v-for="tag in tags"
              :key="tag.id"
              class="px-4 py-3 flex items-center gap-3"
            >
              <!-- Edit mode -->
              <template v-if="editingId === tag.id">
                <input
                  v-model="editName"
                  type="text"
                  class="input flex-1"
                  @keyup.enter="handleSave(tag)"
                  @keyup.esc="cancelEdit"
                />
                <div class="flex flex-wrap items-center gap-1">
                  <button
                    v-for="color in TAG_PALETTE"
                    :key="color"
                    type="button"
                    class="w-5 h-5 rounded-full border-2 transition-transform hover:scale-110"
                    :class="editColor === color ? 'border-slate-900 dark:border-white' : 'border-transparent'"
                    :style="{ backgroundColor: color }"
                    :aria-label="`Use colour ${color}`"
                    :title="color"
                    @click="pickColor('edit', color)"
                  />
                </div>
                <button
                  type="button"
                  class="p-2 rounded-lg bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-300 hover:bg-green-200 dark:hover:bg-green-900/50"
                  title="Save"
                  aria-label="Save tag"
                  :disabled="saving"
                  @click="handleSave(tag)"
                >
                  <CheckIcon class="w-4 h-4" />
                </button>
                <button
                  type="button"
                  class="p-2 rounded-lg text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-700"
                  title="Cancel"
                  aria-label="Cancel edit"
                  @click="cancelEdit"
                >
                  <XMarkIcon class="w-4 h-4" />
                </button>
              </template>

              <!-- View mode -->
              <template v-else>
                <TagChip :tag="tag" />
                <span class="flex-1" />
                <button
                  type="button"
                  class="p-2 rounded-lg text-slate-400 hover:text-slate-600 dark:hover:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700"
                  title="Rename or recolour"
                  aria-label="Edit tag"
                  @click="startEdit(tag)"
                >
                  <PencilSquareIcon class="w-4 h-4" />
                </button>
                <button
                  type="button"
                  class="p-2 rounded-lg text-red-500 hover:text-red-600 dark:hover:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20"
                  title="Delete tag"
                  aria-label="Delete tag"
                  @click="handleDelete(tag)"
                >
                  <TrashIcon class="w-4 h-4" />
                </button>
              </template>
            </li>
          </ul>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 border-t border-slate-200 dark:border-slate-700 flex justify-end">
        <button type="button" class="btn-secondary" @click="emit('close')">Close</button>
      </div>
    </div>
  </div>
</template>
