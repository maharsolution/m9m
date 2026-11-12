<script setup lang="ts">
/**
 * Input — m9m shared primitive.
 *
 * One control covers text, textarea and select (DESIGN.md §7.2):
 * 40px height, 8px radius, 1px border, 2px focus ring. The error
 * state is never colour-only — an error message is always rendered
 * next to the control.
 */
import { computed, useId } from 'vue'

export type InputType = 'text' | 'password' | 'email' | 'number' | 'textarea' | 'select'

const props = withDefaults(
  defineProps<{
    /** Bound value. */
    modelValue?: string | number | null
    /** Control kind. */
    type?: InputType
    /** Field label. */
    label?: string
    /** Placeholder text. */
    placeholder?: string
    /** Helper text below the control. */
    hint?: string
    /** Error text below the control. */
    error?: string
    /** Disables the control. */
    disabled?: boolean
    /** Marks the field as required. */
    required?: boolean
    /** Number of rows for the textarea variant. */
    rows?: number
    /** Native input type for non-textarea controls. */
    inputType?: string
    /** Autocomplete hint for the browser. */
    autocomplete?: string
    /** Name attribute. */
    name?: string
  }>(),
  {
    modelValue: '',
    type: 'text',
    label: undefined,
    placeholder: undefined,
    hint: undefined,
    error: undefined,
    disabled: false,
    required: false,
    rows: 3,
    inputType: 'text',
    autocomplete: undefined,
    name: undefined,
  }
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  blur: [event: FocusEvent]
}>()

const uid = useId()
const controlId = computed(() => `m9m-input-${uid}`)
const describedBy = computed(() => {
  const ids: string[] = []
  if (props.error) ids.push(`${controlId.value}-error`)
  if (props.hint) ids.push(`${controlId.value}-hint`)
  return ids.length ? ids.join(' ') : undefined
})

const baseControlClass = computed(() => [
  'w-full rounded-lg border px-3 text-sm',
  'transition-colors duration-200 ease-out',
  'focus:outline-none focus:ring-2',
  'disabled:opacity-50 disabled:cursor-not-allowed',
  props.error
    ? 'border-red-500 focus:border-red-600 focus:ring-red-500/20'
    : 'border-neutral-300 dark:border-neutral-700 focus:border-neutral-900 dark:focus:border-neutral-100 focus:ring-neutral-900/10 dark:focus:ring-neutral-100/10',
])

const controlClass = computed(() => [
  baseControlClass.value,
  'bg-white dark:bg-neutral-900 text-neutral-950 dark:text-neutral-100',
  'placeholder:text-neutral-400 dark:placeholder:text-neutral-500',
  props.type === 'textarea'
    ? 'min-h-[80px] py-2 leading-relaxed resize-y'
    : 'h-10',
])

/**
 * Native `type` of the plain input. The `type` prop selects the
 * control kind, so text-like kinds map onto the native attribute and
 * everything else falls back to `inputType`.
 */
const nativeType = computed(() => {
  if (props.type === 'password' || props.type === 'email' || props.type === 'number') {
    return props.type
  }
  return props.inputType
})

function handleInput(event: Event) {
  const target = event.target as HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement
  emit('update:modelValue', target.value)
}
</script>

<template>
  <div class="w-full">
    <label
      v-if="label || $slots.label"
      :for="controlId"
      class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300"
    >
      <slot name="label">{{ label }}</slot>
      <span v-if="required" class="text-red-600 dark:text-red-400">*</span>
    </label>

    <textarea
      v-if="type === 'textarea'"
      :id="controlId"
      :value="modelValue ?? ''"
      :rows="rows"
      :placeholder="placeholder"
      :disabled="disabled"
      :required="required"
      :name="name"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
      :class="controlClass"
      @input="handleInput"
      @blur="emit('blur', $event)"
    />

    <select
      v-else-if="type === 'select'"
      :id="controlId"
      :value="modelValue ?? ''"
      :disabled="disabled"
      :required="required"
      :name="name"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
      :class="[controlClass, 'appearance-none bg-no-repeat pr-8']"
      style="background-image: none"
      @input="handleInput"
      @change="handleInput"
      @blur="emit('blur', $event)"
    >
      <slot />
    </select>

    <input
      v-else
      :id="controlId"
      :type="nativeType"
      :value="modelValue ?? ''"
      :placeholder="placeholder"
      :disabled="disabled"
      :required="required"
      :name="name"
      :autocomplete="autocomplete"
      :aria-invalid="error ? 'true' : undefined"
      :aria-describedby="describedBy"
      :class="controlClass"
      @input="handleInput"
      @blur="emit('blur', $event)"
    />

    <p
      v-if="error"
      :id="`${controlId}-error`"
      class="mt-1 text-xs text-red-700 dark:text-red-300"
    >
      <slot name="error">{{ error }}</slot>
    </p>
    <p
      v-else-if="hint || $slots.hint"
      :id="`${controlId}-hint`"
      class="mt-1 text-xs text-neutral-600 dark:text-neutral-400"
    >
      <slot name="hint">{{ hint }}</slot>
    </p>
  </div>
</template>
