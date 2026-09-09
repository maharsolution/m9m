<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  HomeIcon,
  BoltIcon,
  ClockIcon,
  KeyIcon,
  Cog6ToothIcon,
  PlusIcon,
  DocumentDuplicateIcon,
  ChartBarIcon,
  CalendarDaysIcon,
} from '@heroicons/vue/24/outline'
import { useWorkflowStore, useCredentialsStore, useExecutionStore } from '@/stores'

const route = useRoute()
const router = useRouter()
const workflowStore = useWorkflowStore()
const credentialsStore = useCredentialsStore()
const executionStore = useExecutionStore()

interface NavItem {
  name: string
  path: string
  icon: typeof HomeIcon
  badge?: 'count' | 'static'
  staticBadge?: string
}

const navItems: NavItem[] = [
  { name: 'Overview', path: '/', icon: HomeIcon },
  { name: 'Workflows', path: '/workflows', icon: BoltIcon, badge: 'count' },
  { name: 'Credentials', path: '/credentials', icon: KeyIcon, badge: 'count' },
  { name: 'Executions', path: '/executions', icon: ClockIcon, badge: 'count' },
  { name: 'Schedules', path: '/schedules', icon: CalendarDaysIcon },
  { name: 'Templates', path: '/templates', icon: DocumentDuplicateIcon, staticBadge: 'New' },
  { name: 'Performance', path: '/performance', icon: ChartBarIcon },
  { name: 'Settings', path: '/settings', icon: Cog6ToothIcon },
]

const counts = computed(() => ({
  '/workflows': workflowStore.workflows.length,
  '/credentials': credentialsStore.count,
  '/executions': executionStore.executions.length,
} as Record<string, number>))

const isActive = (path: string) => {
  if (path === '/') return route.path === '/'
  return route.path.startsWith(path)
}

const createNewWorkflow = () => {
  router.push('/workflows/new')
}

// Fetch counts on mount so the badges appear immediately.
onMounted(async () => {
  try {
    await Promise.all([
      workflowStore.fetchWorkflows({ limit: 1 }),
      credentialsStore.fetchCredentials(),
      executionStore.fetchExecutions({ limit: 1 }),
    ])
  } catch {
    // Badges can render without counts; no error toast needed.
  }
})
</script>

<template>
  <aside class="w-64 bg-white dark:bg-slate-800 border-r border-slate-200 dark:border-slate-700 flex flex-col">
    <!-- Logo -->
    <div class="h-16 flex items-center px-6 border-b border-slate-200 dark:border-slate-700">
      <div class="flex items-center gap-3">
        <div class="w-8 h-8 bg-gradient-to-br from-violet-500 to-indigo-600 rounded-lg flex items-center justify-center">
          <BoltIcon class="w-5 h-5 text-white" />
        </div>
        <div>
          <span class="text-xl font-bold text-slate-900 dark:text-white">m9m</span>
          <span class="ml-1.5 text-[10px] font-medium text-violet-600 dark:text-violet-400 bg-violet-100 dark:bg-violet-900/30 px-1.5 py-0.5 rounded">Agent Native</span>
        </div>
      </div>
    </div>

    <!-- New Workflow Button -->
    <div class="p-4">
      <button
        @click="createNewWorkflow"
        class="w-full btn-primary flex items-center justify-center gap-2"
      >
        <PlusIcon class="w-5 h-5" />
        <span>New Workflow</span>
      </button>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 px-3 py-2 space-y-1 overflow-y-auto">
      <RouterLink
        v-for="item in navItems"
        :key="item.path"
        :to="item.path"
        class="flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors"
        :class="[
          isActive(item.path)
            ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-400'
            : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:hover:bg-slate-700/50 hover:text-slate-900 dark:hover:text-slate-200'
        ]"
      >
        <component :is="item.icon" class="w-5 h-5" />
        <span class="flex-1">{{ item.name }}</span>
        <!-- Live count badge -->
        <span
          v-if="item.badge === 'count' && counts[item.path] > 0"
          class="text-[10px] font-medium bg-slate-100 dark:bg-slate-700 text-slate-600 dark:text-slate-300 px-1.5 py-0.5 rounded-full min-w-[20px] text-center"
        >
          {{ counts[item.path] }}
        </span>
        <!-- Static badge (e.g. "New" for templates) -->
        <span
          v-else-if="item.staticBadge"
          class="text-[10px] font-medium bg-violet-100 dark:bg-violet-900/50 text-violet-600 dark:text-violet-400 px-1.5 py-0.5 rounded"
        >
          {{ item.staticBadge }}
        </span>
      </RouterLink>
    </nav>

    <!-- Footer -->
    <div class="p-4 border-t border-slate-200 dark:border-slate-700">
      <div class="text-xs text-slate-500 dark:text-slate-400">
        <div class="font-medium">m9m</div>
        <div>v1.0.0 - Agent Native</div>
      </div>
    </div>
  </aside>
</template>
