<script setup lang="ts">
/**
 * Header — n8n parity top bar.
 *
 * 65px tall, 1px bottom border, surface background. Carries the page
 * title (Headline role), the global search, the live connection chip,
 * the theme toggle, the notification bell and the user menu.
 */
import { computed, ref, onMounted, onUnmounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  SunIcon,
  MoonIcon,
  BellIcon,
  UserCircleIcon,
  MagnifyingGlassIcon,
  SignalIcon,
  SignalSlashIcon,
} from '@heroicons/vue/24/outline'
import { Menu, MenuButton, MenuItems, MenuItem } from '@headlessui/vue'
import { useThemeStore, useAuthStore, useExecutionStore } from '@/stores'
import { wsConnection, type ConnectionStatus } from '@/api/websocket'

const route = useRoute()
const router = useRouter()
const themeStore = useThemeStore()
const authStore = useAuthStore()
const executionStore = useExecutionStore()

const pageTitle = computed(() => {
  return (route.meta.title as string) || 'm9m'
})

const hasNotifications = computed(() => executionStore.hasRunningExecutions)

// Global search. Enter hands the query to the workflow list through
// the `q` route parameter; the list owns the actual filtering.
const searchQuery = ref('')

function submitSearch() {
  const query = searchQuery.value.trim()
  if (query) {
    router.push({ path: '/workflows', query: { q: query } })
  } else {
    router.push('/workflows')
  }
}

// WebSocket connection state.
//
// The badge mirrors the state of the push channel:
//   connected        the stream is live
//   connecting       a handshake is running (or a retry is pending)
//   unauthenticated  nobody signed in
//   expired          the session token was rejected
//   disconnected     the backend or the stream is unreachable
const wsStatus = ref<ConnectionStatus>(wsConnection.connectionState)
let connectionCheckInterval: ReturnType<typeof setInterval> | null = null
let unsubscribeStatus: (() => void) | null = null

function updateConnectionState() {
  wsStatus.value = wsConnection.connectionState
}

onMounted(() => {
  updateConnectionState()
  // Check connection state periodically
  connectionCheckInterval = setInterval(updateConnectionState, 1000)

  // Listen for connection state changes
  unsubscribeStatus = wsConnection.onStatus((status) => {
    wsStatus.value = status
  })
})

onUnmounted(() => {
  if (connectionCheckInterval) {
    clearInterval(connectionCheckInterval)
  }
  unsubscribeStatus?.()
})

// The push channel follows the session: it is opened with the session
// and closed with it.
watch(
  () => authStore.isAuthenticated,
  (authenticated) => {
    if (authenticated) {
      wsConnection.connect()
    } else {
      wsConnection.disconnect('unauthenticated')
    }
  }
)

// Connection status text
const connectionStatus = computed(() => {
  switch (wsStatus.value) {
    case 'connected':
      return 'Connected'
    case 'connecting':
      return 'Reconnecting...'
    case 'unauthenticated':
      return 'Not signed in'
    case 'expired':
      return 'Session expired'
    default:
      return 'Disconnected'
  }
})

const connectionStatusTitle = computed(() => {
  switch (wsStatus.value) {
    case 'connected':
      return 'Live updates connected'
    case 'connecting':
      return 'Reconnecting to the server...'
    case 'unauthenticated':
      return 'Sign in to receive live updates'
    case 'expired':
      return 'Your session expired - sign in again'
    default:
      return 'The server is unreachable'
  }
})

const connectionStatusColor = computed(() => {
  switch (wsStatus.value) {
    case 'connected':
      return 'text-green-600 dark:text-green-400'
    case 'connecting':
      return 'text-yellow-700 dark:text-yellow-400'
    default:
      return 'text-red-600 dark:text-red-400'
  }
})

// The chip keeps the look it had: green while the stream is live,
// yellow while it is being re-established. An auth problem gets its
// own red treatment, so a stale session is never mistaken for a
// disconnected server.
const connectionStatusChip = computed(() => {
  switch (wsStatus.value) {
    case 'connected':
      return 'bg-green-50 dark:bg-green-950/40'
    case 'unauthenticated':
    case 'expired':
      return 'bg-red-50 dark:bg-red-950/40'
    default:
      return 'bg-yellow-100 dark:bg-yellow-950/40'
  }
})
</script>

<template>
  <header
    class="flex h-[65px] shrink-0 items-center justify-between gap-4 border-b border-neutral-200 bg-white px-6 dark:border-neutral-800 dark:bg-neutral-900"
  >
    <!-- Left: Page Title -->
    <div class="flex min-w-0 items-center gap-4">
      <h1
        class="truncate text-xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50"
      >
        {{ pageTitle }}
      </h1>
    </div>

    <!-- Center: Search -->
    <div class="hidden min-w-0 flex-1 justify-center md:flex">
      <div class="relative w-full max-w-md">
        <MagnifyingGlassIcon
          class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-neutral-400"
        />
        <input
          v-model="searchQuery"
          type="text"
          placeholder="Search workflows..."
          class="h-10 w-full rounded-lg border border-neutral-200 bg-neutral-50 pl-9 pr-3 text-sm text-neutral-950 transition-colors duration-200 placeholder:text-neutral-400 focus:border-neutral-900 focus:bg-white focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-800 dark:bg-neutral-950 dark:text-neutral-100 dark:placeholder:text-neutral-500 dark:focus:border-neutral-100 dark:focus:bg-neutral-900 dark:focus:ring-neutral-100/10"
          @keyup.enter="submitSearch"
        />
      </div>
    </div>

    <!-- Right: Actions -->
    <div class="flex shrink-0 items-center gap-1.5">
      <!-- WebSocket Connection Status -->
      <div
        class="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2 py-1.5 dark:border-neutral-800"
        :class="connectionStatusChip"
        :title="connectionStatus"
        :aria-label="connectionStatusTitle"
      >
        <span class="relative flex h-2 w-2">
          <span
            v-if="wsStatus === 'connecting'"
            class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
            :class="connectionStatusColor.replace('text-', 'bg-')"
          />
          <span
            class="relative inline-flex h-2 w-2 rounded-full"
            :class="connectionStatusColor.replace('text-', 'bg-')"
          />
        </span>
        <component
          :is="wsStatus === 'connected' ? SignalIcon : SignalSlashIcon"
          class="h-4 w-4"
          :class="connectionStatusColor"
        />
      </div>

      <!-- Theme Toggle -->
      <button
        type="button"
        @click="themeStore.toggleTheme"
        class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-neutral-500 transition-colors duration-200 hover:bg-neutral-100 hover:text-neutral-900 focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-neutral-100 dark:focus-visible:outline-neutral-100"
        :title="themeStore.isDark ? 'Switch to light mode' : 'Switch to dark mode'"
      >
        <SunIcon v-if="themeStore.isDark" class="h-5 w-5" />
        <MoonIcon v-else class="h-5 w-5" />
      </button>

      <!-- Notifications -->
      <button
        type="button"
        class="relative inline-flex h-8 w-8 items-center justify-center rounded-lg text-neutral-500 transition-colors duration-200 hover:bg-neutral-100 hover:text-neutral-900 focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-neutral-100 dark:focus-visible:outline-neutral-100"
        title="Notifications"
      >
        <BellIcon class="h-5 w-5" />
        <span
          v-if="hasNotifications"
          class="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-brand-500"
        />
      </button>

      <!-- User Menu -->
      <Menu as="div" class="relative">
        <MenuButton
          class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-neutral-500 transition-colors duration-200 hover:bg-neutral-100 hover:text-neutral-900 focus-visible:outline-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 dark:text-neutral-400 dark:hover:bg-neutral-800 dark:hover:text-neutral-100 dark:focus-visible:outline-neutral-100"
        >
          <UserCircleIcon class="h-6 w-6" />
        </MenuButton>

        <transition
          enter-active-class="transition duration-100 ease-out"
          enter-from-class="transform scale-95 opacity-0"
          enter-to-class="transform scale-100 opacity-100"
          leave-active-class="transition duration-75 ease-in"
          leave-from-class="transform scale-100 opacity-100"
          leave-to-class="transform scale-95 opacity-0"
        >
          <MenuItems class="absolute right-0 z-50 mt-2 w-48 origin-top-right rounded-lg border border-neutral-200 bg-white py-1 shadow-sm focus:outline-none dark:border-neutral-800 dark:bg-neutral-900">
            <MenuItem v-slot="{ active }">
              <RouterLink
                to="/settings"
                class="block px-4 py-2 text-sm text-neutral-700 dark:text-neutral-200"
                :class="{ 'bg-neutral-100 dark:bg-neutral-800': active }"
              >
                Settings
              </RouterLink>
            </MenuItem>
            <MenuItem v-if="authStore.isAuthenticated" v-slot="{ active }">
              <button
                @click="authStore.logout"
                class="block w-full px-4 py-2 text-left text-sm text-neutral-700 dark:text-neutral-200"
                :class="{ 'bg-neutral-100 dark:bg-neutral-800': active }"
              >
                Sign out
              </button>
            </MenuItem>
            <MenuItem v-else v-slot="{ active }">
              <RouterLink
                to="/login"
                class="block px-4 py-2 text-sm text-neutral-700 dark:text-neutral-200"
                :class="{ 'bg-neutral-100 dark:bg-neutral-800': active }"
              >
                Sign in
              </RouterLink>
            </MenuItem>
          </MenuItems>
        </transition>
      </Menu>
    </div>
  </header>
</template>
