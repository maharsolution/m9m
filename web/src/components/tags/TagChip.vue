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

const sizeClass = computed(() =>
  props.size === 'md' ? 'px-2.5 py-1 text-sm' : 'px-2 py-0.5 text-xs'
)
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full border font-medium transition-colors"
    :class="[
      sizeClass,
      hex
        ? 'text-slate-700 dark:text-slate-200'
        : 'bg-slate-100 dark:bg-slate-700 border-slate-200 dark:border-slate-600 text-slate-600 dark:text-slate-300',
      clickable ? 'cursor-pointer hover:brightness-95 dark:hover:brightness-110' : '',
    ]"
    :style="hex ? chipStyle : undefined"
    :title="tag.name"
  >
    <span
      class="w-2 h-2 rounded-full shrink-0"
      :class="hex ? '' : 'bg-slate-400'"
      :style="swatchStyle"
    />
    <span class="truncate max-w-[12rem]">{{ tag.name }}</span>

    <button
      v-if="removable"
      type="button"
      class="ml-0.5 rounded-full p-0.5 hover:bg-black/10 dark:hover:bg-white/10 transition-colors"
      :aria-label="`Remove tag ${tag.name}`"
      :title="`Remove tag ${tag.name}`"
      @click.stop="emit('remove', tag)"
    >
      <XMarkIcon class="w-3 h-3" />
    </button>
  </span>
</template>
