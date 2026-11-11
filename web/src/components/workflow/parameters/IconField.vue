<script setup lang="ts">
import { computed, type Component } from 'vue'
import {
  BellIcon,
  BoltIcon,
  CalendarIcon,
  CircleStackIcon,
  ClockIcon,
  CloudIcon,
  CodeBracketIcon,
  Cog6ToothIcon,
  DocumentTextIcon,
  FolderIcon,
  GlobeAltIcon,
  HeartIcon,
  LinkIcon,
  PhotoIcon,
  ShieldCheckIcon,
  StarIcon,
  TableCellsIcon,
  TagIcon,
  TrashIcon,
  UserIcon,
} from '@heroicons/vue/24/outline'
import type { NodeProperty } from '@/types/node'

/**
 * Icon parameter renderer (`icon`).
 *
 * n8n stores an icon as its `IconOrEmoji` shape:
 * `{ "type": "icon", "value": "<icon name>" }`.
 *
 * The metadata lists the offered icons as lucide-style names
 * ("document-text", "cog-6-tooth"); the map below translates the
 * curated set into Heroicons so the panel renders a real glyph. A
 * name outside the map falls back to its text.
 */
interface IconValue {
  type: 'icon'
  value: string
}

/** lucide-style name → Heroicon component. */
const ICON_COMPONENTS: Record<string, Component> = {
  user: UserIcon,
  folder: FolderIcon,
  'document-text': DocumentTextIcon,
  calendar: CalendarIcon,
  clock: ClockIcon,
  tag: TagIcon,
  bolt: BoltIcon,
  cloud: CloudIcon,
  'code-bracket': CodeBracketIcon,
  'globe-alt': GlobeAltIcon,
  heart: HeartIcon,
  star: StarIcon,
  trash: TrashIcon,
  'cog-6-tooth': Cog6ToothIcon,
  link: LinkIcon,
  photo: PhotoIcon,
  'shield-check': ShieldCheckIcon,
  bell: BellIcon,
  'circle-stack': CircleStackIcon,
  'table-cells': TableCellsIcon,
}

const props = withDefaults(
  defineProps<{
    property: NodeProperty
    modelValue?: unknown
    disabled?: boolean
  }>(),
  {
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: IconValue): void
}>()

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** Normalise whatever is stored (or missing) into the n8n shape. */
function normalize(raw: unknown): IconValue {
  if (typeof raw === 'string') {
    return { type: 'icon', value: raw }
  }
  if (!isPlainObject(raw)) {
    return { type: 'icon', value: '' }
  }
  return {
    type: 'icon',
    value: typeof raw.value === 'string' ? raw.value : '',
  }
}

const state = computed<IconValue>(() => normalize(props.modelValue))

/** The icons offered by the metadata. */
const icons = computed(() => props.property.values ?? [])

/** The Heroicon component of the selected icon (if it is known). */
const selectedComponent = computed<Component | null>(() =>
  ICON_COMPONENTS[state.value.value] ?? null,
)

function selectIcon(name: string) {
  emit('update:modelValue', { type: 'icon', value: name })
}

function onCustomInput(event: Event) {
  const value = (event.target as HTMLInputElement).value
  emit('update:modelValue', { type: 'icon', value })
}
</script>

<template>
  <div class="space-y-2">
    <div
      v-if="icons.length"
      class="grid grid-cols-6 gap-1"
    >
      <button
        v-for="icon in icons"
        :key="icon.value"
        type="button"
        class="flex items-center justify-center h-8 rounded border transition-colors"
        :class="state.value === icon.value
          ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/30'
          : 'border-slate-200 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-700'"
        :title="icon.name"
        :disabled="disabled"
        @click="selectIcon(icon.value)"
      >
        <component
          :is="ICON_COMPONENTS[icon.value]"
          v-if="ICON_COMPONENTS[icon.value]"
          class="w-4 h-4 text-slate-600 dark:text-slate-300"
        />
        <span
          v-else
          class="text-[10px] leading-none text-slate-600 dark:text-slate-300"
        >
          {{ icon.name.slice(0, 2) }}
        </span>
      </button>
    </div>

    <div class="flex items-center gap-2">
      <span class="flex items-center justify-center w-8 h-8 shrink-0 rounded border border-slate-200 dark:border-slate-600">
        <component
          :is="selectedComponent"
          v-if="selectedComponent"
          class="w-4 h-4 text-slate-600 dark:text-slate-300"
        />
        <span
          v-else-if="state.value"
          class="text-[10px] leading-none text-slate-500 dark:text-slate-400"
        >
          {{ state.value.slice(0, 3) }}
        </span>
      </span>

      <input
        :value="state.value"
        type="text"
        class="input flex-1"
        placeholder="Custom icon name"
        :disabled="disabled"
        @input="onCustomInput"
      />
    </div>
  </div>
</template>
