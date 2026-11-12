<script setup lang="ts">
import { computed } from 'vue'
import { XMarkIcon } from '@heroicons/vue/24/outline'
import type { Tag } from '@/types/tag'
import { normalizeHexColor } from '@/types/tag'

const props = withDefaults(defineProps<{
  /** The tag to render. */
  tag: Tag
  /** Show a remove (×) button. */
  removable?: boolean
  /** Chip size. */
  size?: 'sm' | 'md'
  /** Render the chip as a button. */
  clickable?: boolean
}>(), {
  removable: false,
  size: 'sm',
  clickable: false,
})

const emit = defineEmits<{
  remove: [tag: Tag]
  click: [tag: Tag]
}>()

const hex = computed(() => normalizeHexColor(props.tag.color))

/** Tinted background + border derived from the tag colour. */
const chipStyle = computed(() => {
  if (!hex.value) return {}
  return {
    backgroundColor: `${hex.value}1F`,
    borderColor: `${hex.value}4D`,
  }
})

const swatchStyle = computed(() => {
  if (!hex.value) return {}
  return { backgroundColor: hex.value }
})

// n8n tag treatment: 20px chip, 8px radius, 12px label.
const sizeClass = computed(() =>
  props.size === 'md' ? 'h-6 px-2 text-xs' : 'h-5 px-2 text-2xs'
)
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-lg border font-medium transition-colors duration-200"
    :class="[
      sizeClass,
      hex
        ? 'text-neutral-800 dark:text-neutral-100'
        : 'border-neutral-200 bg-neutral-100 text-neutral-600 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-300',
      clickable ? 'cursor-pointer hover:brightness-95 dark:hover:brightness-110' : '',
    ]"
    :style="hex ? chipStyle : undefined"
    :title="tag.name"
  >
    <span
      class="h-1.5 w-1.5 shrink-0 rounded-full"
      :class="hex ? '' : 'bg-neutral-400'"
      :style="swatchStyle"
    />
    <span class="truncate max-w-[12rem]">{{ tag.name }}</span>

    <button
      v-if="removable"
      type="button"
      class="ml-0.5 rounded-sm p-0.5 transition-colors hover:bg-black/10 dark:hover:bg-white/10"
      :aria-label="`Remove tag ${tag.name}`"
      :title="`Remove tag ${tag.name}`"
      @click.stop="emit('remove', tag)"
    >
      <XMarkIcon class="h-3 w-3" />
    </button>
  </span>
</template>
