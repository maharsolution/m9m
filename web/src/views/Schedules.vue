<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import {
  PlusIcon,
  CalendarDaysIcon,
  FunnelIcon,
  MagnifyingGlassIcon,
} from '@heroicons/vue/24/outline'
import { useScheduleStore, useWorkflowStore } from '@/stores'
import type { Schedule, ScheduleCreateRequest, ScheduleUpdateRequest } from '@/types/schedule'
import ScheduleList from '@/components/schedule/ScheduleList.vue'
import ScheduleForm from '@/components/schedule/ScheduleForm.vue'
import ScheduleHistory from '@/components/schedule/ScheduleHistory.vue'
import Button from '@/components/common/Button.vue'

const scheduleStore = useScheduleStore()
const workflowStore = useWorkflowStore()

// UI State
const showForm = ref(false)
const showHistory = ref(false)
const editingSchedule = ref<Schedule | null>(null)
const historySchedule = ref<Schedule | null>(null)

// Filters
const searchQuery = ref('')
const statusFilter = ref<'all' | 'enabled' | 'disabled'>('all')
const workflowFilter = ref('')

onMounted(async () => {
  await Promise.all([
    scheduleStore.fetchSchedules(),
    workflowStore.fetchWorkflows(),
  ])
})

const filteredSchedules = computed(() => {
  let schedules = scheduleStore.schedules

  if (statusFilter.value === 'enabled') {
    schedules = schedules.filter((s) => s.enabled)
  } else if (statusFilter.value === 'disabled') {
    schedules = schedules.filter((s) => !s.enabled)
  }

  if (workflowFilter.value) {
    schedules = schedules.filter((s) => s.workflowId === workflowFilter.value)
  }

  if (searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    schedules = schedules.filter(
      (s) =>
        s.workflowName?.toLowerCase().includes(query) ||
        s.cronExpression.includes(query)
    )
  }

  return schedules
})

// Workflow list for form
const workflowOptions = computed(() =>
  workflowStore.workflows.map((w) => ({ id: w.id, name: w.name }))
)

// Actions
function openCreateForm() {
  editingSchedule.value = null
  showForm.value = true
}

function openEditForm(schedule: Schedule) {
  editingSchedule.value = schedule
  showForm.value = true
}

async function handleSubmit(data: ScheduleCreateRequest | ScheduleUpdateRequest) {
  try {
    if (editingSchedule.value) {
      await scheduleStore.updateSchedule(editingSchedule.value.id, data as ScheduleUpdateRequest)
    } else {
      await scheduleStore.createSchedule(data as ScheduleCreateRequest)
    }
    showForm.value = false
    editingSchedule.value = null
  } catch (e) {
    console.error('Failed to save schedule:', e)
  }
}

async function handleDelete(schedule: Schedule) {
  if (confirm(`Are you sure you want to delete this schedule for "${schedule.workflowName || 'Unknown'}"?`)) {
    try {
      await scheduleStore.deleteSchedule(schedule.id)
    } catch (e) {
      console.error('Failed to delete schedule:', e)
    }
  }
}

async function handleToggle(schedule: Schedule) {
  try {
    await scheduleStore.toggleScheduleEnabled(schedule.id)
  } catch (e) {
    console.error('Failed to toggle schedule:', e)
  }
}

async function openHistory(schedule: Schedule) {
  historySchedule.value = schedule
  showHistory.value = true
  await scheduleStore.fetchScheduleHistory(schedule.id)
}
</script>

<template>
  <div>
    <!-- Page header -->
    <div class="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="flex items-center gap-3 text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
          <CalendarDaysIcon class="h-6 w-6 text-brand-500" />
          Schedules
        </h1>
        <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
          Automate workflow execution with cron schedules
        </p>
      </div>
      <Button
        variant="primary"
        size="sm"
        @click="openCreateForm"
      >
        <template #icon>
          <PlusIcon class="h-4 w-4" />
        </template>
        <span>Create schedule</span>
      </Button>
    </div>

    <!-- Stats Cards -->
    <div class="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div class="rounded-lg border border-neutral-200 bg-white p-4 dark:border-neutral-800 dark:bg-neutral-900">
        <p class="text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
          Total
        </p>
        <p class="mt-1 text-2xl font-semibold text-neutral-950 dark:text-neutral-50">
          {{ scheduleStore.total }}
        </p>
      </div>
      <div class="rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-900 dark:bg-green-950/50">
        <p class="text-2xs font-medium uppercase tracking-wide text-green-700 dark:text-green-400">
          Active
        </p>
        <p class="mt-1 text-2xl font-semibold text-green-800 dark:text-green-300">
          {{ scheduleStore.enabledSchedules.length }}
        </p>
      </div>
      <div class="rounded-lg border border-neutral-200 bg-white p-4 dark:border-neutral-800 dark:bg-neutral-900">
        <p class="text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
          Inactive
        </p>
        <p class="mt-1 text-2xl font-semibold text-neutral-700 dark:text-neutral-300">
          {{ scheduleStore.disabledSchedules.length }}
        </p>
      </div>
      <div class="rounded-lg border border-brand-200 bg-brand-50 p-4 dark:border-brand-900 dark:bg-brand-950/50">
        <p class="text-2xs font-medium uppercase tracking-wide text-brand-700 dark:text-brand-400">
          Upcoming
        </p>
        <p class="mt-1 text-2xl font-semibold text-brand-800 dark:text-brand-300">
          {{ scheduleStore.schedulesWithUpcomingRuns.length }}
        </p>
      </div>
    </div>

    <!-- Filters -->
    <div class="mb-6 flex flex-wrap items-center gap-3">
      <!-- Search -->
      <div class="relative w-full max-w-sm">
        <MagnifyingGlassIcon class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-neutral-400" />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search schedules…"
          class="h-9 w-full rounded-lg border border-neutral-300 bg-white pl-9 pr-4 text-sm text-neutral-950 transition-colors duration-200 placeholder:text-neutral-400 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10"
        >
      </div>

      <!-- Status Filter -->
      <div class="flex items-center gap-2">
        <FunnelIcon class="h-4 w-4 text-neutral-400" />
        <select
          v-model="statusFilter"
          class="h-9 rounded-lg border border-neutral-300 bg-white px-3 text-sm text-neutral-950 transition-colors duration-200 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10"
        >
          <option value="all">All Status</option>
          <option value="enabled">Active</option>
          <option value="disabled">Inactive</option>
        </select>
      </div>

      <!-- Workflow Filter -->
      <select
        v-model="workflowFilter"
        class="h-9 rounded-lg border border-neutral-300 bg-white px-3 text-sm text-neutral-950 transition-colors duration-200 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10"
      >
        <option value="">All Workflows</option>
        <option
          v-for="workflow in workflowStore.workflows"
          :key="workflow.id"
          :value="workflow.id"
        >
          {{ workflow.name }}
        </option>
      </select>
    </div>

    <!-- Schedule List -->
    <ScheduleList
      :schedules="filteredSchedules"
      :loading="scheduleStore.loading"
      @edit="openEditForm"
      @delete="handleDelete"
      @toggle="handleToggle"
      @view-history="openHistory"
    />

    <!-- Create/Edit Form Modal -->
    <ScheduleForm
      v-if="showForm"
      :schedule="editingSchedule"
      :workflows="workflowOptions"
      @submit="handleSubmit"
      @cancel="showForm = false; editingSchedule = null"
    />

    <!-- History Modal -->
    <ScheduleHistory
      v-if="showHistory"
      :history="scheduleStore.currentHistory"
      :loading="scheduleStore.loading"
      @close="showHistory = false; historySchedule = null"
    />
  </div>
</template>
