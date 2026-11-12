<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { CRON_PRESETS } from '@/types/schedule'
import { ClockIcon, CalendarIcon, InformationCircleIcon } from '@heroicons/vue/24/outline'

const props = defineProps<{
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const mode = ref<'preset' | 'custom'>('preset')
const selectedPreset = ref('')

// Custom cron parts
const minute = ref('*')
const hour = ref('*')
const dayOfMonth = ref('*')
const month = ref('*')
const dayOfWeek = ref('*')

// Parse initial value
function parseExpression(expr: string) {
  const parts = expr.split(' ')
  if (parts.length === 5) {
    minute.value = parts[0]
    hour.value = parts[1]
    dayOfMonth.value = parts[2]
    month.value = parts[3]
    dayOfWeek.value = parts[4]
  }
}

// Initialize
if (props.modelValue) {
  parseExpression(props.modelValue)
  const preset = CRON_PRESETS.find(p => p.value === props.modelValue)
  if (preset) {
    selectedPreset.value = preset.value
    mode.value = 'preset'
  } else {
    mode.value = 'custom'
  }
}

// Build expression from parts
const customExpression = computed(() => {
  return `${minute.value} ${hour.value} ${dayOfMonth.value} ${month.value} ${dayOfWeek.value}`
})

// Watch for changes
watch([minute, hour, dayOfMonth, month, dayOfWeek], () => {
  if (mode.value === 'custom') {
    emit('update:modelValue', customExpression.value)
  }
})

watch(selectedPreset, (val) => {
  if (val && mode.value === 'preset') {
    emit('update:modelValue', val)
    parseExpression(val)
  }
})

watch(mode, (val) => {
  if (val === 'preset' && selectedPreset.value) {
    emit('update:modelValue', selectedPreset.value)
  } else if (val === 'custom') {
    emit('update:modelValue', customExpression.value)
  }
})

// Get human-readable description
function getCronDescription(expr: string): string {
  const parts = expr.split(' ')
  if (parts.length !== 5) return 'Invalid expression'

  const [min, hr, dom, mon, dow] = parts

  // Simple pattern matching
  if (expr === '* * * * *') return 'Every minute'
  if (min.startsWith('*/') && hr === '*' && dom === '*' && mon === '*' && dow === '*') {
    return `Every ${min.slice(2)} minutes`
  }
  if (min === '0' && hr.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') {
    return `Every ${hr.slice(2)} hours`
  }
  if (min === '0' && hr === '0' && dom === '*' && mon === '*' && dow === '*') {
    return 'Daily at midnight'
  }
  if (min === '0' && hr !== '*' && dom === '*' && mon === '*' && dow === '*') {
    return `Daily at ${hr}:00`
  }
  if (min === '0' && hr === '0' && dom === '*' && mon === '*' && dow !== '*') {
    const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
    const dayNum = parseInt(dow)
    if (!isNaN(dayNum) && dayNum >= 0 && dayNum <= 6) {
      return `Weekly on ${days[dayNum]}`
    }
  }
  if (min === '0' && hr === '0' && dom !== '*' && mon === '*' && dow === '*') {
    return `Monthly on day ${dom}`
  }

  return `At ${min} ${hr} ${dom} ${mon} ${dow}`
}

const cronDescription = computed(() => {
  const expr = mode.value === 'preset' ? selectedPreset.value : customExpression.value
  return getCronDescription(expr)
})

// Field options
const minuteOptions = [
  { label: 'Every minute', value: '*' },
  { label: 'Every 5 minutes', value: '*/5' },
  { label: 'Every 10 minutes', value: '*/10' },
  { label: 'Every 15 minutes', value: '*/15' },
  { label: 'Every 30 minutes', value: '*/30' },
  { label: 'At minute 0', value: '0' },
]

const hourOptions = [
  { label: 'Every hour', value: '*' },
  { label: 'Every 2 hours', value: '*/2' },
  { label: 'Every 6 hours', value: '*/6' },
  { label: 'Every 12 hours', value: '*/12' },
  ...Array.from({ length: 24 }, (_, i) => ({ label: `At ${i}:00`, value: String(i) })),
]

const dayOfMonthOptions = [
  { label: 'Every day', value: '*' },
  ...Array.from({ length: 31 }, (_, i) => ({ label: `Day ${i + 1}`, value: String(i + 1) })),
]

const monthOptions = [
  { label: 'Every month', value: '*' },
  { label: 'January', value: '1' },
  { label: 'February', value: '2' },
  { label: 'March', value: '3' },
  { label: 'April', value: '4' },
  { label: 'May', value: '5' },
  { label: 'June', value: '6' },
  { label: 'July', value: '7' },
  { label: 'August', value: '8' },
  { label: 'September', value: '9' },
  { label: 'October', value: '10' },
  { label: 'November', value: '11' },
  { label: 'December', value: '12' },
]

const dayOfWeekOptions = [
  { label: 'Every day', value: '*' },
  { label: 'Sunday', value: '0' },
  { label: 'Monday', value: '1' },
  { label: 'Tuesday', value: '2' },
  { label: 'Wednesday', value: '3' },
  { label: 'Thursday', value: '4' },
  { label: 'Friday', value: '5' },
  { label: 'Saturday', value: '6' },
  { label: 'Weekdays', value: '1-5' },
  { label: 'Weekends', value: '0,6' },
]

const CONTROL_CLASS =
  'h-9 w-full rounded-lg border border-neutral-300 bg-white px-3 text-sm text-neutral-950 transition-colors duration-200 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10'
</script>

<template>
  <div class="space-y-4">
    <!-- Mode Toggle -->
    <div class="flex rounded-lg border border-neutral-200 bg-neutral-100 p-1 dark:border-neutral-800 dark:bg-neutral-950">
      <button
        type="button"
        class="flex flex-1 items-center justify-center rounded-md px-4 py-1.5 text-sm font-medium transition-colors duration-200"
        :class="mode === 'preset'
          ? 'bg-white text-neutral-950 shadow-xs dark:bg-neutral-800 dark:text-neutral-50'
          : 'text-neutral-600 hover:text-neutral-950 dark:text-neutral-400 dark:hover:text-neutral-100'"
        @click="mode = 'preset'"
      >
        <ClockIcon class="mr-2 h-4 w-4" />
        Presets
      </button>
      <button
        type="button"
        class="flex flex-1 items-center justify-center rounded-md px-4 py-1.5 text-sm font-medium transition-colors duration-200"
        :class="mode === 'custom'
          ? 'bg-white text-neutral-950 shadow-xs dark:bg-neutral-800 dark:text-neutral-50'
          : 'text-neutral-600 hover:text-neutral-950 dark:text-neutral-400 dark:hover:text-neutral-100'"
        @click="mode = 'custom'"
      >
        <CalendarIcon class="mr-2 h-4 w-4" />
        Custom
      </button>
    </div>

    <!-- Preset Selection -->
    <div
      v-if="mode === 'preset'"
      class="space-y-2"
    >
      <div class="grid grid-cols-2 gap-2">
        <button
          v-for="preset in CRON_PRESETS"
          :key="preset.value"
          type="button"
          class="rounded-lg border px-4 py-3 text-left transition-colors duration-200"
          :class="selectedPreset === preset.value
            ? 'border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-950/50 dark:text-brand-300'
            : 'border-neutral-200 text-neutral-700 hover:border-neutral-300 hover:bg-neutral-50 dark:border-neutral-700 dark:text-neutral-300 dark:hover:border-neutral-600 dark:hover:bg-neutral-800'"
          @click="selectedPreset = preset.value"
        >
          <div class="text-sm font-medium">{{ preset.label }}</div>
          <code class="font-mono text-2xs text-neutral-500 dark:text-neutral-400">{{ preset.value }}</code>
        </button>
      </div>
    </div>

    <!-- Custom Builder -->
    <div
      v-else
      class="space-y-4"
    >
      <div class="grid grid-cols-5 gap-3">
        <!-- Minute -->
        <div>
          <label class="mb-1 block text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            Minute
          </label>
          <select
            v-model="minute"
            :class="CONTROL_CLASS"
          >
            <option
              v-for="opt in minuteOptions"
              :key="opt.value"
              :value="opt.value"
            >
              {{ opt.label }}
            </option>
          </select>
        </div>

        <!-- Hour -->
        <div>
          <label class="mb-1 block text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            Hour
          </label>
          <select
            v-model="hour"
            :class="CONTROL_CLASS"
          >
            <option
              v-for="opt in hourOptions"
              :key="opt.value"
              :value="opt.value"
            >
              {{ opt.label }}
            </option>
          </select>
        </div>

        <!-- Day of Month -->
        <div>
          <label class="mb-1 block text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            Day
          </label>
          <select
            v-model="dayOfMonth"
            :class="CONTROL_CLASS"
          >
            <option
              v-for="opt in dayOfMonthOptions"
              :key="opt.value"
              :value="opt.value"
            >
              {{ opt.label }}
            </option>
          </select>
        </div>

        <!-- Month -->
        <div>
          <label class="mb-1 block text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            Month
          </label>
          <select
            v-model="month"
            :class="CONTROL_CLASS"
          >
            <option
              v-for="opt in monthOptions"
              :key="opt.value"
              :value="opt.value"
            >
              {{ opt.label }}
            </option>
          </select>
        </div>

        <!-- Day of Week -->
        <div>
          <label class="mb-1 block text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            Weekday
          </label>
          <select
            v-model="dayOfWeek"
            :class="CONTROL_CLASS"
          >
            <option
              v-for="opt in dayOfWeekOptions"
              :key="opt.value"
              :value="opt.value"
            >
              {{ opt.label }}
            </option>
          </select>
        </div>
      </div>

      <!-- Raw Expression Input -->
      <div>
        <label class="mb-1 block text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
          Cron Expression
        </label>
        <input
          :value="customExpression"
          type="text"
          :class="[CONTROL_CLASS, 'font-mono']"
          placeholder="* * * * *"
          @input="(e) => {
            const val = (e.target as HTMLInputElement).value
            parseExpression(val)
            emit('update:modelValue', val)
          }"
        >
      </div>
    </div>

    <!-- Description -->
    <div class="flex items-center gap-2 rounded-lg border border-neutral-200 bg-neutral-50 p-3 dark:border-neutral-800 dark:bg-neutral-950">
      <InformationCircleIcon class="h-4 w-4 text-brand-500" />
      <span class="text-sm text-neutral-700 dark:text-neutral-300">
        {{ cronDescription }}
      </span>
    </div>
  </div>
</template>
