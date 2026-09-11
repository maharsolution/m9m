<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
import { getBuildInfo, type BuildInfo } from '@/api/buildInfo'

const route = useRoute()
const router = useRouter()
const workflowStore = useWorkflowStore()
const credentialsStore = useCredentialsStore()
const executionStore = useExecutionStore()

// buildInfo is fetched once on mount from /api/v1/version so the
// footer chip below reflects the exact source identity stamped into the
// running binary by the Dockerfile (-ldflags -X main.Commit=...).
// Surfaced as "<version> @ <commit> • <buildDate>" so a user can
// distinguish "I just rebuilt the container and the chip changed" from
// "the chip still shows the previous commit — old image is running".
//
// A null buildInfo (network error / backend down) is treated as
// "unknown" so the footer doesn't go blank.
const buildInfo = ref<BuildInfo | null>(null)

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

  // Pull the running binary's source identity in parallel so the
  // footer chip below reflects the latest rebuild. We swallow errors
  // silently — when the backend isn't reachable (offline dev box,
  // CORS misconfig, etc.) the footer just shows the static "v1.0.0"
  // label rather than going blank.
  try {
    buildInfo.value = await getBuildInfo()
  } catch {
    buildInfo.value = null
  }
})

// Footer fields rendered in the bottom of the sidebar.
//
// `versionLabel` — what to show next to the m9m logo in the footer.
//   * When buildInfo loaded successfully: "<version> @ <commit>"
//     (e.g. "v1.0.0 @ abc1234"). Short enough to fit in a 280px wide
//     sidebar, and the commit hash lets the user cross-reference the
//     change list ("which push is the container on?").
//   * When buildInfo is missing (backend offline): fall back to the
//     static "v1.0.0 - Agent Native" label so the sidebar never goes
//     blank.
//
// `buildDateLabel` — small text under `versionLabel` showing when
//   the binary was built. Truncated to "YYYY-MM-DD HH:MM" so it fits
//   the footer width; the full ISO timestamp is also rendered in the
//   `title` attribute as a tooltip for users who want the exact
//   second.
const versionLabel = computed(() => {
  const info = buildInfo.value
  if (!info) return 'v1.0.0 - Agent Native'
  const v = info.version && info.version !== 'unknown' ? info.version : 'dev'
  const c = info.commit && info.commit !== 'unknown' ? info.commit : 'unknown'
  return `${v} @ ${c}`
})

const buildDateLabel = computed(() => {
  const info = buildInfo.value
  if (!info || !info.buildDate || info.buildDate === 'unknown') return ''
  // Trim "2026-09-11T07:00:00Z" → "2026-09-11 07:00 UTC" for the chip,
  // and keep the raw ISO string in `title` for hover.
  const pretty = info.buildDate.replace('T', ' ').replace(/Z$/, ' UTC')
  return pretty
})

const buildDateTitle = computed(() => {
  const info = buildInfo.value
  if (!info || !info.buildDate || info.buildDate === 'unknown') return ''
  return `Built ${info.buildDate}`
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
    <!-- Footer: shows the running binary's source identity so users
         can tell at a glance whether the container is on the latest
         push. The `versionLabel` is "<version> @ <commit-short>"
         (e.g. "v1.0.0 @ abc1234"); `buildDateLabel` underneath shows
         the UTC build timestamp. When the /version endpoint is
         unreachable (backend offline / CORS error / dev env) the
         labels fall back to the static "v1.0.0 - Agent Native" so
         the sidebar never goes blank. -->
    <div class="p-4 border-t border-slate-200 dark:border-slate-700">
      <div class="text-xs text-slate-500 dark:text-slate-400">
        <div class="font-medium">m9m</div>
        <div
          class="font-mono"
          :title="buildDateTitle || 'Build identity unavailable'"
        >
          {{ versionLabel }}
        </div>
        <div v-if="buildDateLabel" class="text-[10px] text-slate-400 dark:text-slate-500 mt-0.5 font-mono">
          {{ buildDateLabel }}
        </div>
      </div>
    </div>
  </aside>
</template>
