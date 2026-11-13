<script setup lang="ts">
/**
 * Sidebar — n8n parity nav rail.
 *
 * 42px collapsed / 200px expanded (n8n `$sidebar-width` /
 * `$sidebar-expanded-width`). The rail is icon-only by default, opens
 * on hover, and the logo pins it open — the pinned state is persisted
 * in localStorage so the preference survives a reload.
 *
 * Every navigation entry, route, badge count, the "New workflow"
 * action and the build-id footer are preserved.
 */
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
import { useWorkflowStore, useCredentialsStore, useExecutionStore, useWorkflowEditorStore } from '@/stores'
import { getBuildInfo, type BuildInfo } from '@/api/buildInfo'
import Tooltip from '@/components/common/Tooltip.vue'

const route = useRoute()
const router = useRouter()
const workflowStore = useWorkflowStore()
const credentialsStore = useCredentialsStore()
const executionStore = useExecutionStore()
const workflowEditorStore = useWorkflowEditorStore()

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
  // The editor counts the request: a new workflow opened while the
  // editor already shows one does not change the route, so the canvas
  // would keep the previous draft.
  workflowEditorStore.requestNewWorkflow()
  if (route.path !== '/workflows/new') {
    router.push('/workflows/new')
  }
}

// ---------------------------------------------------------------------------
// Collapsed / expanded state
//
//   collapsed : 42px  — icon only
//   expanded : 200px — icon + label + badges
//
// The rail expands on hover. Clicking the logo pins it open; the pin
// is persisted so the operator's choice survives a reload.
// ---------------------------------------------------------------------------
const STORAGE_KEY = 'm9m:sidebar-expanded'

const pinned = ref(false)
const hovering = ref(false)

const expanded = computed(() => pinned.value || hovering.value)

function readPinned(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'true'
  } catch {
    return false
  }
}

function writePinned(value: boolean) {
  try {
    localStorage.setItem(STORAGE_KEY, String(value))
  } catch {
    // localStorage unavailable (private mode) — the preference is
    // session-only in that case.
  }
}

function togglePinned() {
  pinned.value = !pinned.value
  writePinned(pinned.value)
}

// Fetch counts on mount so the badges appear immediately.
onMounted(async () => {
  pinned.value = readPinned()

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
//     (e.g. "v1.0.0 @ abc1234"). Short enough to fit in the 200px
//     rail, and the commit hash lets the user cross-reference the
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
  <aside
    class="h-full shrink-0 overflow-hidden border-r border-neutral-200 bg-white transition-[width] duration-200 ease-out dark:border-neutral-800 dark:bg-neutral-900"
    :style="{ width: expanded ? '200px' : '42px' }"
    :aria-expanded="expanded"
    @mouseenter="hovering = true"
    @mouseleave="hovering = false"
  >
    <!-- Brand / logo — click pins the rail open or closed -->
    <button
      type="button"
      class="flex h-[65px] w-full shrink-0 items-center gap-2.5 border-b border-neutral-200 px-2 text-left transition-colors duration-200 hover:bg-neutral-50 focus-visible:outline-none focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-neutral-950 dark:border-neutral-800 dark:hover:bg-neutral-800 dark:focus-visible:outline-neutral-100"
      :title="pinned ? 'Collapse navigation' : 'Expand navigation'"
      :aria-label="pinned ? 'Collapse navigation' : 'Expand navigation'"
      @click="togglePinned"
    >
      <span
        class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-brand-500 text-white"
      >
        <BoltIcon class="h-4 w-4" />
      </span>
      <span
        v-if="expanded"
        class="flex min-w-0 flex-col leading-none"
      >
        <span class="truncate text-base font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
          m9m
        </span>
        <span class="mt-0.5 truncate text-3xs font-medium uppercase tracking-wide text-brand-600 dark:text-brand-400">
          Agent Native
        </span>
      </span>
    </button>

    <!-- New workflow -->
    <div class="shrink-0 px-2 pt-3 pb-1">
      <button
        type="button"
        class="flex h-9 w-full items-center justify-center gap-2 rounded-lg bg-brand-500 px-2 text-sm font-medium text-white transition-colors duration-200 hover:bg-brand-600 active:bg-brand-700 focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 dark:focus-visible:outline-neutral-100"
        :title="'New Workflow'"
        @click="createNewWorkflow"
      >
        <PlusIcon class="h-4 w-4 shrink-0" />
        <span v-if="expanded" class="truncate">New Workflow</span>
      </button>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 overflow-y-auto overflow-x-hidden px-2 py-2">
      <ul class="space-y-0.5">
        <li v-for="item in navItems" :key="item.path">
          <RouterLink
            :to="item.path"
            class="group relative flex h-9 items-center gap-2.5 rounded-lg px-2 text-sm font-medium transition-colors duration-200 ease-out focus-visible:outline-none focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-neutral-950 dark:focus-visible:outline-neutral-100"
            :class="
              isActive(item.path)
                ? 'bg-brand-50 text-brand-700 dark:bg-brand-950/40 dark:text-brand-300'
                : 'text-neutral-600 hover:bg-neutral-100 hover:text-neutral-950 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-neutral-100'
            "
            :title="expanded ? undefined : item.name"
          >
            <span
              v-if="isActive(item.path)"
              class="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-brand-500"
              aria-hidden="true"
            />
            <component :is="item.icon" class="h-5 w-5 shrink-0" />
            <span v-if="expanded" class="min-w-0 flex-1 truncate">{{ item.name }}</span>
            <!-- Live count badge -->
            <span
              v-if="item.badge === 'count' && counts[item.path] > 0"
              class="shrink-0 rounded-full bg-neutral-100 px-1.5 py-0.5 text-3xs font-medium tabular-nums text-neutral-700 dark:bg-neutral-800 dark:text-neutral-200"
            >
              {{ counts[item.path] }}
            </span>
            <!-- Static badge (e.g. "New" for templates) -->
            <span
              v-else-if="item.staticBadge"
              class="shrink-0 rounded-full bg-brand-50 px-1.5 py-0.5 text-3xs font-medium text-brand-700 dark:bg-brand-950/50 dark:text-brand-300"
            >
              {{ item.staticBadge }}
            </span>
          </RouterLink>
        </li>
      </ul>
    </nav>

    <!-- Footer: shows the running binary's source identity so users
         can tell at a glance whether the container is on the latest
         push. The `versionLabel` is "<version> @ <commit-short>"
         (e.g. "v1.0.0 @ abc1234"); `buildDateLabel` underneath shows
         the UTC build timestamp. When the /version endpoint is
         unreachable (backend offline / CORS error / dev env) the
         labels fall back to the static "v1.0.0 - Agent Native" so
         the sidebar never goes blank. -->
    <div class="shrink-0 border-t border-neutral-200 dark:border-neutral-800">
      <Tooltip
        v-if="!expanded"
        :text="`${versionLabel}${buildDateLabel ? ` · ${buildDateLabel}` : ''}`"
        placement="right"
      >
        <div
          class="flex h-10 w-full items-center justify-center"
          :title="buildDateTitle || 'Build identity unavailable'"
        >
          <span
            class="h-1.5 w-1.5 rounded-full bg-brand-500"
            aria-hidden="true"
          />
          <span class="sr-only">{{ versionLabel }}</span>
        </div>
      </Tooltip>

      <div v-else class="px-3 py-2.5">
        <div class="flex items-center gap-1.5">
          <span class="h-1.5 w-1.5 rounded-full bg-brand-500" aria-hidden="true" />
          <span class="text-2xs font-semibold uppercase tracking-wide text-neutral-950 dark:text-neutral-50">
            m9m
          </span>
        </div>
        <div
          class="mt-0.5 truncate font-mono text-3xs text-neutral-600 dark:text-neutral-400"
          :title="buildDateTitle || 'Build identity unavailable'"
        >
          {{ versionLabel }}
        </div>
        <div
          v-if="buildDateLabel"
          class="mt-0.5 truncate font-mono text-3xs text-neutral-500 dark:text-neutral-500"
        >
          {{ buildDateLabel }}
        </div>
      </div>
    </div>
  </aside>
</template>
