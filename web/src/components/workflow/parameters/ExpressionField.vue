<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { ExclamationTriangleIcon, LightBulbIcon } from '@heroicons/vue/24/outline'
import type { NodeProperty } from '@/types/node'
import type { FieldTree } from '@/lib/expressionFields'
import {
  applySuggestionAt,
  createPreviewContext,
  detectExpressionPosition,
  evaluateExpression,
  evaluateValue,
  formatPreview,
  FUNCTION_CATALOGUE,
  selectDisplayError,
  suggestForPosition,
  VARIABLES,
  type AvailableNode,
  type ExpressionPosition,
  type Suggestion,
  type SuggestionSources,
} from './expression/index'

/**
 * Rich expression editor for node parameters.
 *
 * m9m evaluates a parameter that starts with `=` as an expression, e.g.
 * `={{ $json.user.id }}`. This component gives the user the same
 * affordances n8n has:
 *
 * - a plain-text mode and an expression mode (`fx` toggle),
 * - live evaluation with a preview row,
 * - an inline error banner with the offending span highlighted,
 * - a keyboard-navigable autocomplete for variables, nodes, item
 *   fields and functions.
 *
 * The evaluation happens in the browser with the hand-written engine
 * in `./expression` — no `eval`, no `new Function`, no `with`.
 */

const props = withDefaults(
  defineProps<{
    /** Current parameter value. */
    modelValue?: string | number | boolean | null
    /** Property metadata (label, placeholder, rows). */
    property?: NodeProperty
    /** Nodes that can be referenced through `$node["Name"]`. */
    availableNodes?: AvailableNode[]
    /** Field names of the current item (`$json.<field>`). */
    itemFields?: string[]
    /** Field names per node name (`$node["Name"].json.<field>`). */
    nodeFields?: Record<string, string[]>
    /** The nested fields of the current item (`$json.address.city`). */
    itemTree?: FieldTree
    /** The nested fields per node name (`$node["Name"].json.address`). */
    nodeTrees?: Record<string, FieldTree>
    /** Optional `$json` payload used for the live preview. */
    sampleInput?: unknown
    /** Disabled state (read-only panels). */
    disabled?: boolean
  }>(),
  {
    modelValue: '',
    property: undefined,
    availableNodes: () => [],
    itemFields: () => [],
    nodeFields: () => ({}),
    itemTree: undefined,
    nodeTrees: () => ({}),
    sampleInput: undefined,
    disabled: false,
  },
)

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
}>()

/** Matches the canonical m9m expression envelope `={{ … }}`. */
const ENVELOPE = /^\s*=\s*\{\{([\s\S]*)\}\}\s*$/

/** Prefix used when the expression mode serialises its value. */
const EXPRESSION_PREFIX = '={{ '

/** Suffix used when the expression mode serialises its value. */
const EXPRESSION_SUFFIX = ' }}'

const inputId = useId()
const listboxId = `${inputId}-suggestions`

/** `true` while the user edits an expression. */
const expressionMode = ref(false)

/** Text shown in the editor. */
const text = ref('')

/** Caret offset inside {@link text}. */
const caret = ref(0)

/** The editor element — the caret is placed explicitly after an insert. */
const inputRef = ref<HTMLInputElement | HTMLTextAreaElement | null>(null)

/** Autocomplete suggestions for the current prefix. */
const suggestions = ref<Suggestion[]>([])

/** Index of the highlighted suggestion. */
const activeSuggestion = ref(0)

/** Whether the suggestion dropdown is visible. */
const suggestionsOpen = ref(false)

/**
 * Set right after an insertion.
 *
 * The browser fires a `keyup` / `click` after the insert, and that
 * event would open the list again on the inserted text.
 */
const suppressRefresh = ref(false)

/** The caret position the current suggestions were built for. */
const position = ref<ExpressionPosition | null>(null)

/** Label of the field. */
const label = computed(() => props.property?.displayName ?? 'Expression')

/** Placeholder shown in the editor. */
const placeholder = computed(() => props.property?.placeholder ?? '')

/** Number of rows for the textarea (`0` renders a single-line input). */
const rows = computed(() => props.property?.typeOptions?.rows ?? 0)

/** Whether a multi-line editor is used. */
const multiline = computed(() => rows.value > 1)

/** Serialised value of the field. */
const value = computed(() =>
  expressionMode.value
    ? `${EXPRESSION_PREFIX}${text.value}${EXPRESSION_SUFFIX}`
    : text.value,
)

/** Preview context derived from the `sampleInput` prop. */
const context = computed(() => createPreviewContext(props.sampleInput))

/**
 * Result of evaluating the current content.
 *
 * In expression mode the body is evaluated on its own, so error offsets
 * point at the character the user actually sees. In plain-text mode the
 * whole value is evaluated, which also covers interpolated values such
 * as `Hello {{ $json.name }}`.
 */
const evaluation = computed(() =>
  expressionMode.value
    ? evaluateExpression(text.value, context.value)
    : evaluateValue(text.value, context.value),
)

/** The current parse error, if any. */
const error = computed(() => (evaluation.value.ok ? null : evaluation.value.error))

/** True when the value contains at least one expression. */
const hasExpression = computed(
  () => expressionMode.value || /\{\{[\s\S]*\}\}/.test(text.value),
)

/** Preview text, only available when a sample input was supplied. */
const preview = computed(() => {
  if (props.sampleInput === undefined) return null
  if (!evaluation.value.ok) return null
  if (!hasExpression.value) return null
  if (expressionMode.value && text.value.trim() === '') return null
  return formatPreview(evaluation.value.value)
})

/** The offending source, split around the highlighted span. */
const errorSnippet = computed(() => {
  const current = error.value
  if (!current) return null
  const source = text.value
  const start = Math.max(0, Math.min(current.start, source.length))
  const end = Math.max(start, Math.min(current.end, source.length))
  const before = source.slice(Math.max(0, start - 24), start)
  const middle = source.slice(start, end)
  const after = source.slice(end, end + 24)
  return { before, middle, after }
})

/** True when the dropdown should be rendered. */
const showSuggestions = computed(
  () => suggestionsOpen.value && suggestions.value.length > 0,
)

/**
 * The error the editor shows.
 *
 * The rules live in `selectDisplayError` so they can be unit tested:
 *
 * - A half-written expression is not an error while the autocomplete
 *   is open — n8n shows the suggestion list instead of a red banner.
 * - The *unterminated* error is suppressed while the caret sits in an
 *   expression that is not closed yet, even when the suggestion list
 *   is empty: `{{$json.bdy` is unfinished, not broken.
 * - A runtime error is only shown when the editor has data to evaluate
 *   the expression against. Without a preview there is nothing to
 *   resolve `$json` with, and a red banner would only report that the
 *   editor knows nothing — not that the expression is wrong.
 */
const displayError = computed(() =>
  selectDisplayError(error.value, {
    text: text.value,
    caret: caret.value,
    suggestionsOpen: showSuggestions.value,
    hasSampleInput: props.sampleInput !== undefined,
  }),
)

/** True when the field holds an expression. */
const isExpression = computed(() => expressionMode.value)

/** The data the autocomplete completes with. */
const sources = computed<SuggestionSources>(() => ({
  variables: VARIABLES,
  nodes: props.availableNodes,
  functions: FUNCTION_CATALOGUE,
  itemFields: props.itemFields,
  nodeFields: props.nodeFields,
  itemTree: props.itemTree,
  nodeTrees: props.nodeTrees,
}))

/** Sync the editor state when the model changes from the outside. */
watch(
  () => props.modelValue,
  (next) => {
    const raw = next === null || next === undefined ? '' : String(next)
    // Ignore the echo of our own update.
    if (raw === value.value) return

    const match = raw.match(ENVELOPE)
    if (match) {
      expressionMode.value = true
      text.value = match[1].trim()
      return
    }
    expressionMode.value = false
    text.value = raw
  },
  { immediate: true },
)

/** Push the current value to the parent. */
function publish() {
  emit('update:modelValue', value.value)
}

/** Handle typing inside the editor. */
function onInput(event: Event) {
  const target = event.target as HTMLInputElement | HTMLTextAreaElement
  text.value = target.value
  caret.value = target.selectionStart ?? target.value.length
  suppressRefresh.value = false
  publish()
  refreshSuggestions()
}

/** Track the caret so the suggestions stay in sync. */
function onSelect(event: Event) {
  const target = event.target as HTMLInputElement | HTMLTextAreaElement
  caret.value = target.selectionStart ?? target.value.length

  // Right after an insertion the list has to stay closed — the next
  // `keyup` / `click` would open it again on the inserted text.
  if (suppressRefresh.value) {
    suppressRefresh.value = false
    return
  }

  refreshSuggestions()
}

/** Toggle between plain-text and expression mode. */
function toggleExpression() {
  expressionMode.value = !expressionMode.value
  publish()
  refreshSuggestions()
}

/**
 * Recompute the suggestions for the current caret position.
 *
 * The list opens as soon as the caret is inside an expression — right
 * after `{{`, behind a dot, or in the middle of a token — and closes
 * again when the caret leaves the expression.
 */
function refreshSuggestions() {
  const current = detectExpressionPosition(text.value, caret.value, {
    insideExpression: expressionMode.value,
  })
  position.value = current

  if (!current.inExpression) {
    suggestions.value = []
    suggestionsOpen.value = false
    activeSuggestion.value = 0
    return
  }

  suggestions.value = suggestForPosition(current, sources.value)
  suggestionsOpen.value = suggestions.value.length > 0
  activeSuggestion.value = 0
}

/** Insert the highlighted suggestion at the caret. */
function applyActiveSuggestion() {
  const suggestion = suggestions.value[activeSuggestion.value]
  const current = position.value
  if (!suggestion || !current) return
  const result = applySuggestionAt(text.value, caret.value, current, suggestion.insertText)
  text.value = result.text
  caret.value = result.caret
  publish()
  suggestionsOpen.value = false
  suggestions.value = []
  activeSuggestion.value = 0
  suppressRefresh.value = true

  // The browser keeps the caret where it was when the value of the
  // field is replaced. The caret has to follow the insertion, or the
  // next character lands in the wrong place.
  void nextTick(() => {
    const element = inputRef.value
    if (!element || typeof element.setSelectionRange !== 'function') return
    element.setSelectionRange(result.caret, result.caret)
  })
}

/** Keyboard handling for the editor and the suggestion dropdown. */
function onKeydown(event: KeyboardEvent) {
  if (showSuggestions.value) {
    // The dropdown owns the keyboard while it is open — the editor
    // (and the canvas behind it) must not react to these keys.
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      event.stopPropagation()
      activeSuggestion.value =
        (activeSuggestion.value + 1) % suggestions.value.length
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      event.stopPropagation()
      activeSuggestion.value =
        (activeSuggestion.value - 1 + suggestions.value.length) %
        suggestions.value.length
      return
    }
    if (event.key === 'Enter' || event.key === 'Tab') {
      event.preventDefault()
      event.stopPropagation()
      applyActiveSuggestion()
      return
    }
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      suggestionsOpen.value = false
      suggestions.value = []
      activeSuggestion.value = 0
      // The browser fires a `keyup` right after the `keydown`. Without
      // this the list would be reopened by `onSelect` and the caret
      // would still sit inside the expression.
      suppressRefresh.value = true
      return
    }
    return
  }

  if (event.key === 'Escape') {
    suggestionsOpen.value = false
  }
}

/** Insert a suggestion that was clicked in the dropdown. */
function onSuggestionClick(suggestion: Suggestion) {
  activeSuggestion.value = suggestions.value.indexOf(suggestion)
  applyActiveSuggestion()
}
</script>

<template>
  <div class="space-y-1">
    <div class="flex items-start gap-2">
      <div class="relative flex-1">
        <label :for="inputId" class="sr-only">{{ label }}</label>
        <textarea
          v-if="multiline"
          :id="inputId"
          ref="inputRef"
          v-model="text"
          :rows="rows"
          :placeholder="placeholder"
          :disabled="disabled"
          class="input font-mono text-sm"
          :class="isExpression ? 'ring-1 ring-primary-400 dark:ring-primary-500' : ''"
          spellcheck="false"
          autocomplete="off"
          role="combobox"
          aria-autocomplete="list"
          :aria-expanded="showSuggestions"
          :aria-controls="listboxId"
          aria-haspopup="listbox"
          @input="onInput"
          @click="onSelect"
          @keyup="onSelect"
          @keydown="onKeydown"
        />
        <input
          v-else
          :id="inputId"
          ref="inputRef"
          v-model="text"
          type="text"
          :placeholder="placeholder"
          :disabled="disabled"
          class="input"
          :class="isExpression ? 'font-mono text-sm ring-1 ring-primary-400 dark:ring-primary-500' : ''"
          spellcheck="false"
          autocomplete="off"
          role="combobox"
          aria-autocomplete="list"
          :aria-expanded="showSuggestions"
          :aria-controls="listboxId"
          aria-haspopup="listbox"
          @input="onInput"
          @click="onSelect"
          @keyup="onSelect"
          @keydown="onKeydown"
        />

        <!-- Autocomplete dropdown -->
        <div
          v-if="showSuggestions"
          :id="listboxId"
          role="listbox"
          class="absolute z-30 mt-1 w-full max-h-64 overflow-y-auto rounded-lg border border-default bg-surface shadow-lg"
        >
          <button
            v-for="(suggestion, index) in suggestions"
            :key="`${suggestion.kind}-${suggestion.label}`"
            type="button"
            role="option"
            :aria-selected="index === activeSuggestion"
            class="flex w-full items-baseline gap-2 px-3 py-1.5 text-left text-sm"
            :class="
              index === activeSuggestion
                ? 'bg-hover text-primary'
                : 'text-secondary'
            "
            @mousedown.prevent="onSuggestionClick(suggestion)"
          >
            <span class="font-mono text-xs shrink-0">{{ suggestion.label }}</span>
            <span class="truncate text-xs text-tertiary">{{
              suggestion.detail
            }}</span>
          </button>
          <p
            class="border-t border-subtle px-3 py-1 text-[11px] text-tertiary"
          >
            ↑ ↓ to navigate · Enter to insert · Esc to close
          </p>
        </div>
      </div>

      <button
        type="button"
        class="px-2 py-1 text-xs font-mono rounded border border-slate-300 dark:border-slate-600 text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-700 shrink-0"
        :class="
          isExpression
            ? 'bg-primary-50 dark:bg-slate-700 text-primary-600 dark:text-primary-400'
            : ''
        "
        :title="
          isExpression
            ? 'Switch back to a literal value'
            : 'Use an expression ({{ ... }})'
        "
        :aria-pressed="isExpression"
        :disabled="disabled"
        @click="toggleExpression"
      >
        fx
      </button>
    </div>

    <!-- Parse error -->
    <p
      v-if="displayError"
      role="alert"
      class="flex items-start gap-1.5 text-xs text-red-600 dark:text-red-400"
    >
      <ExclamationTriangleIcon class="w-4 h-4 mt-0.5 shrink-0" aria-hidden="true" />
      <span>
        {{ displayError.message }}
        <span
          v-if="errorSnippet"
          class="font-mono text-[11px] text-red-500 dark:text-red-300"
        >
          <span>{{ errorSnippet.before }}</span
          ><mark class="bg-red-500/30 rounded px-0.5 text-red-700 dark:text-red-200">{{
            errorSnippet.middle
          }}</mark
          ><span>{{ errorSnippet.after }}</span>
        </span>
      </span>
    </p>

    <!-- Live preview -->
    <p
      v-else-if="preview !== null"
      class="flex items-start gap-1.5 text-xs text-slate-500 dark:text-slate-400"
    >
      <LightBulbIcon class="w-4 h-4 mt-0.5 shrink-0" aria-hidden="true" />
      <span class="min-w-0">
        Result:
        <span class="font-mono break-all text-slate-700 dark:text-slate-200">{{
          preview
        }}</span>
      </span>
    </p>
  </div>
</template>
