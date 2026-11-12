<script setup lang="ts">
import { ref, computed } from 'vue'
import type { Schedule, ScheduleCreateRequest, ScheduleUpdateRequest } from '@/types/schedule'
import { TIMEZONES } from '@/types/schedule'
import CronExpressionBuilder from './CronExpressionBuilder.vue'
import { XMarkIcon, ClockIcon } from '@heroicons/vue/24/outline'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'

interface Workflow {
  id: string
  name: string
}

const props = defineProps<{
  schedule?: Schedule | null
  workflows: Workflow[]
}>()

const emit = defineEmits<{
  submit: [data: ScheduleCreateRequest | ScheduleUpdateRequest]
  cancel: []
}>()

// Form data
const workflowId = ref(props.schedule?.workflowId || '')
const cronExpression = ref(props.schedule?.cronExpression || '0 * * * *')
const timezone = ref(props.schedule?.timezone || 'UTC')
const enabled = ref(props.schedule?.enabled ?? true)
const maxRuns = ref(props.schedule?.maxRuns || 0)
const maxDuration = ref(props.schedule?.maxDuration || 0)

const isEditing = computed(() => !!props.schedule)

const formTitle = computed(() => isEditing.value ? 'Edit Schedule' : 'Create Schedule')

const isValid = computed(() => {
  if (!isEditing.value && !workflowId.value) return false
  if (!cronExpression.value) return false
  return true
})

const CONTROL_CLASS =
  'h-10 w-full rounded-lg border border-neutral-300 bg-white px-3 text-sm text-neutral-950 transition-colors duration-200 placeholder:text-neutral-400 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 disabled:opacity-50 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10'

function handleSubmit() {
  if (!isValid.value) return

  if (isEditing.value) {
    const data: ScheduleUpdateRequest = {
      cronExpression: cronExpression.value,
      timezone: timezone.value,
      enabled: enabled.value,
      maxRuns: maxRuns.value,
      maxDuration: maxDuration.value || undefined,
    }
    emit('submit', data)
  } else {
    const data: ScheduleCreateRequest = {
      workflowId: workflowId.value,
      cronExpression: cronExpression.value,
      timezone: timezone.value,
      enabled: enabled.value,
      maxRuns: maxRuns.value,
      maxDuration: maxDuration.value || undefined,
    }
    emit('submit', data)
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[oklch(100%_0_89.88/0.8)] p-4 backdrop-blur-[2px] dark:bg-black/60">
    <div class="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900">
      <!-- Header -->
      <div class="flex items-center justify-between border-b border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <div class="flex items-center gap-3">
          <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-950/50 dark:text-brand-400">
            <ClockIcon class="h-4 w-4" />
          </div>
          <h2 class="text-lg font-semibold text-neutral-950 dark:text-neutral-50">
            {{ formTitle }}
          </h2>
        </div>
        <IconButton
          label="Close"
          @click="emit('cancel')"
        >
          <XMarkIcon class="h-4 w-4" />
        </IconButton>
      </div>

      <!-- Form -->
      <form
        class="flex-1 space-y-6 overflow-y-auto p-6"
        @submit.prevent="handleSubmit"
      >
        <!-- Workflow Selection (only for create) -->
        <div v-if="!isEditing">
          <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
            Workflow
          </label>
          <select
            v-model="workflowId"
            :class="CONTROL_CLASS"
            required
          >
            <option value="" disabled>Select a workflow</option>
            <option
              v-for="workflow in workflows"
              :key="workflow.id"
              :value="workflow.id"
            >
              {{ workflow.name }}
            </option>
          </select>
        </div>

        <!-- Cron Expression Builder -->
        <div>
          <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
            Schedule
          </label>
          <CronExpressionBuilder v-model="cronExpression" />
        </div>

        <!-- Timezone -->
        <div>
          <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
            Timezone
          </label>
          <select
            v-model="timezone"
            :class="CONTROL_CLASS"
          >
            <option
              v-for="tz in TIMEZONES"
              :key="tz"
              :value="tz"
            >
              {{ tz }}
            </option>
          </select>
        </div>

        <!-- Max Runs -->
        <div>
          <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
            Maximum runs
          </label>
          <input
            v-model.number="maxRuns"
            type="number"
            min="0"
            :class="CONTROL_CLASS"
            placeholder="0 for unlimited"
          >
          <p class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
            Set to 0 for unlimited runs
          </p>
        </div>

        <!-- Max Duration -->
        <div>
          <label class="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-neutral-700 dark:text-neutral-300">
            Maximum duration (seconds)
          </label>
          <input
            v-model.number="maxDuration"
            type="number"
            min="0"
            :class="CONTROL_CLASS"
            placeholder="0 for no limit"
          >
          <p class="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
            Maximum execution duration before timeout. Set to 0 for no limit.
          </p>
        </div>

        <!-- Enabled Toggle -->
        <div class="flex items-center justify-between rounded-lg border border-neutral-200 bg-neutral-50 p-4 dark:border-neutral-800 dark:bg-neutral-950">
          <div>
            <h4 class="text-sm font-medium text-neutral-950 dark:text-neutral-50">
              Enable Schedule
            </h4>
            <p class="mt-0.5 text-xs text-neutral-500 dark:text-neutral-400">
              When enabled, the workflow will run according to the schedule
            </p>
          </div>
          <button
            type="button"
            :aria-pressed="enabled"
            class="relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500"
            :class="enabled ? 'bg-brand-500' : 'bg-neutral-200 dark:bg-neutral-700'"
            @click="enabled = !enabled"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow transition duration-200"
              :class="enabled ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
        </div>
      </form>

      <!-- Footer -->
      <div class="flex justify-end gap-3 border-t border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <Button
          variant="secondary"
          size="sm"
          @click="emit('cancel')"
        >
          Cancel
        </Button>
        <Button
          variant="primary"
          size="sm"
          :disabled="!isValid"
          @click="handleSubmit"
        >
          {{ isEditing ? 'Save changes' : 'Create schedule' }}
        </Button>
      </div>
    </div>
  </div>
</template>
