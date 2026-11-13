<script setup lang="ts">
import { computed, ref } from 'vue'
import { CheckIcon, ClipboardDocumentIcon } from '@heroicons/vue/24/outline'
import { buildWebhookUrls, currentPublicBase } from './webhookUrls'

/**
 * Webhook URL panel for the Webhook node.
 *
 * n8n shows both endpoints of a Webhook node in the parameter panel:
 *
 *   - Production URL — `<base>/webhook/<path>` (requires an ACTIVE
 *     workflow).
 *   - Test URL       — `<base>/webhook-test/<path>` (works while the
 *     flow is still being designed — that is how the input / output of
 *     a flow is captured).
 *
 * The panel is read-only: the path itself stays editable through the
 * regular Path parameter field.
 */
const props = withDefaults(
  defineProps<{
    /** Current value of the Webhook node's `path` parameter. */
    path?: string
    /** Public base — defaults to the origin the editor is served from. */
    baseUrl?: string
  }>(),
  {
    path: '',
    baseUrl: '',
  },
)

const urls = computed(() => buildWebhookUrls(props.baseUrl || currentPublicBase(), props.path))

const copied = ref<'production' | 'test' | null>(null)

async function copy(kind: 'production' | 'test', url: string) {
  try {
    await navigator.clipboard.writeText(url)
    copied.value = kind
    setTimeout(() => {
      if (copied.value === kind) copied.value = null
    }, 2000)
  } catch (error) {
    console.error('Failed to copy webhook URL:', error)
  }
}
</script>

<template>
  <div class="space-y-2 rounded-md border border-slate-200 dark:border-slate-700 p-2">
    <div class="space-y-1">
      <label class="block text-xs font-medium text-slate-600 dark:text-slate-300">
        Production URL
      </label>
      <div class="flex items-center gap-2">
        <input
          type="text"
          class="input font-mono text-xs"
          readonly
          :value="urls.productionUrl"
        >
        <button
          type="button"
          class="shrink-0 rounded p-1 text-slate-500 hover:text-primary-600 dark:text-slate-400 dark:hover:text-primary-400"
          title="Copy production URL"
          @click="copy('production', urls.productionUrl)"
        >
          <component
            :is="copied === 'production' ? CheckIcon : ClipboardDocumentIcon"
            class="w-4 h-4"
          />
        </button>
      </div>
      <p class="text-xs text-slate-500 dark:text-slate-400">
        Only runs while the workflow is active.
      </p>
    </div>

    <div class="space-y-1">
      <label class="block text-xs font-medium text-slate-600 dark:text-slate-300">
        Test URL
      </label>
      <div class="flex items-center gap-2">
        <input
          type="text"
          class="input font-mono text-xs"
          readonly
          :value="urls.testUrl"
        >
        <button
          type="button"
          class="shrink-0 rounded p-1 text-slate-500 hover:text-primary-600 dark:text-slate-400 dark:hover:text-primary-400"
          title="Copy test URL"
          @click="copy('test', urls.testUrl)"
        >
          <component
            :is="copied === 'test' ? CheckIcon : ClipboardDocumentIcon"
            class="w-4 h-4"
          />
        </button>
      </div>
      <p class="text-xs text-slate-500 dark:text-slate-400">
        Runs while the workflow is inactive — use it to capture input and output during design.
      </p>
    </div>
  </div>
</template>
