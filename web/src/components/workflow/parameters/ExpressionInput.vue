<script setup lang="ts">
import { computed } from 'vue'
import { isExpressionValue } from './parameterPath'

/**
 * Text input that understands m9m expressions.
 *
 * m9m (like n8n) evaluates a parameter that starts with `=` as an
 * expression, e.g. `={{ $json.foo }}`. The input accepts a plain value
 * and offers a single toggle that switches between the two modes — no
 * fake behaviour, no expression evaluation in the browser.
 */
const props = withDefaults(
  defineProps<{
    modelValue?: unknown
    placeholder?: string
    /** `text` for a normal input, `password` for secret values. */
    inputType?: 'text' | 'password'
    /** Number of rows; 0 (default) renders a single-line input. */
    rows?: number
    /** Disabled state (read-only panels). */
    disabled?: boolean
  }>(),
  {
    placeholder: '',
    inputType: 'text',
    rows: 0,
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
}>()

const text = computed(() => (typeof props.modelValue === 'string' ? props.modelValue : ''))

const isExpression = computed(() => isExpressionValue(props.modelValue))

/** Example shown in the hint — kept in script so the template does not
 *  parse the `{{ … }}` braces as an interpolation. */
const expressionExample = '={{ $json.field }}'

function onInput(event: Event) {
  const target = event.target as HTMLInputElement | HTMLTextAreaElement
  emit('update:modelValue', target.value)
}

/** Toggle between a literal value and an expression (`={{ … }}`). */
function toggleExpression() {
  if (isExpression.value) {
    emit('update:modelValue', text.value.replace(/^=/, ''))
    return
  }
  emit('update:modelValue', `=${text.value}`)
}
</script>

<template>
  <div class="space-y-1">
    <div class="flex items-start gap-2">
      <textarea
        v-if="rows > 0"
        :value="text"
        :rows="rows"
        :placeholder="placeholder"
        :disabled="disabled"
        class="input font-mono text-sm flex-1"
        @input="onInput"
      />
      <input
        v-else
        :value="text"
        :type="inputType"
        :placeholder="placeholder"
        :disabled="disabled"
        class="input flex-1"
        :class="isExpression ? 'font-mono text-sm' : ''"
        @input="onInput"
      />
      <button
        type="button"
        class="px-2 py-1 text-xs font-mono rounded border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700 shrink-0"
        :class="isExpression ? 'bg-primary-50 dark:bg-slate-700 text-primary-600 dark:text-primary-400' : ''"
        :title="isExpression ? 'Switch back to a literal value' : 'Use an expression ({{ ... }})'"
        :disabled="disabled"
        @click="toggleExpression"
      >
        fx
      </button>
    </div>
    <p
      v-if="isExpression"
      class="text-xs text-slate-500 dark:text-slate-400"
    >
      Expression — resolved at runtime, e.g.
      <code class="font-mono">{{ expressionExample }}</code>
    </p>
  </div>
</template>
