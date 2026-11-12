<script setup lang="ts">
/**
 * Dialog — m9m shared primitive.
 *
 * Modal with an `oklch(100% 0 89.88 / 0.8)` scrim, a focus trap,
 * Escape-to-close and header / body / footer slots (DESIGN.md §7.8).
 */
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    /** Open state (use `v-model:open`). */
    open: boolean
    /** Dialog title rendered in the header slot area. */
    title?: string
    /** Maximum width of the panel. */
    size?: 'sm' | 'md' | 'lg' | 'xl'
    /** Close when the scrim is clicked. */
    closeOnScrim?: boolean
    /** Close when Escape is pressed. */
    closeOnEscape?: boolean
  }>(),
  {
    title: undefined,
    size: 'md',
    closeOnScrim: true,
    closeOnEscape: true,
  }
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  close: []
}>()

const panelRef = ref<HTMLElement | null>(null)

const SIZE_CLASSES: Record<string, string> = {
  sm: 'max-w-sm',
  md: 'max-w-lg',
  lg: 'max-w-2xl',
  xl: 'max-w-5xl',
}

function requestClose() {
  emit('update:open', false)
  emit('close')
}

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.closeOnEscape) {
    event.stopPropagation()
    requestClose()
    return
  }

  if (event.key !== 'Tab') return

  const panel = panelRef.value
  if (!panel) return

  const focusables = Array.from(
    panel.querySelectorAll<HTMLElement>(FOCUSABLE),
  ).filter((el) => el.offsetParent !== null || el === document.activeElement)

  if (focusables.length === 0) {
    event.preventDefault()
    panel.focus()
    return
  }

  const first = focusables[0]
  const last = focusables[focusables.length - 1]

  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    await nextTick()
    const panel = panelRef.value
    if (!panel) return
    const target = panel.querySelector<HTMLElement>(FOCUSABLE)
    ;(target ?? panel).focus()
  },
)

onBeforeUnmount(() => {
  /* nothing to clean up — the scrim is rendered by this component */
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-center justify-center p-4"
      @keydown="handleKeydown"
    >
      <!-- Scrim: oklch(100% 0 89.88 / 0.8) -->
      <div
        class="absolute inset-0 bg-[oklch(100%_0_89.88/0.8)] backdrop-blur-[1px]"
        @click="closeOnScrim ? requestClose() : undefined"
      />

      <div
        ref="panelRef"
        role="dialog"
        aria-modal="true"
        :aria-label="title"
        tabindex="-1"
        class="relative flex max-h-[90vh] w-full flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl outline-none dark:border-neutral-800 dark:bg-neutral-900"
        :class="SIZE_CLASSES[size]"
      >
        <div
          v-if="title || $slots.header"
          class="flex items-start justify-between gap-4 border-b border-neutral-200 px-5 py-4 dark:border-neutral-800"
        >
          <slot name="header">
            <h2 class="text-base font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
              {{ title }}
            </h2>
          </slot>
          <button
            v-if="closeOnScrim || closeOnEscape"
            type="button"
            class="-mr-1 -mt-1 inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-neutral-500 transition-colors duration-200 hover:bg-neutral-100 hover:text-neutral-900 focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 dark:hover:bg-neutral-800 dark:hover:text-neutral-100 dark:focus-visible:outline-neutral-100"
            title="Close"
            aria-label="Close"
            @click="requestClose"
          >
            <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div class="flex-1 overflow-y-auto px-5 py-4">
          <slot />
        </div>

        <div
          v-if="$slots.footer"
          class="flex items-center justify-end gap-2 border-t border-neutral-200 px-5 py-4 dark:border-neutral-800"
        >
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Teleport>
</template>
