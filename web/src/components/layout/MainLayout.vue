<script setup lang="ts">
/**
 * MainLayout — n8n parity app shell.
 *
 *   * Left nav rail   : 42px collapsed / 200px expanded (Sidebar.vue)
 *   * Top bar         : 65px, 1px bottom border (Header.vue)
 *   * Banner slot     : 48px, optional
 *   * Content         : 1280px max-width container (n8n
 *                       `--content-container--width`)
 *
 * The workflow editor is a child route of this layout and owns the
 * full viewport, so it renders without the content container — every
 * other view is constrained to the 1280px container.
 */
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import Sidebar from './Sidebar.vue'
import Header from './Header.vue'
import { useNodesStore, useAuthStore } from '@/stores'
import { wsConnection } from '@/api/websocket'

const route = useRoute()
const nodesStore = useNodesStore()
const authStore = useAuthStore()

/**
 * The editor (`/workflows/new` and `/workflows/:id`) fills the whole
 * viewport — it brings its own toolbar and canvas. The execution
 * detail (`/executions/:id`) does the same: the graph panel spans the
 * full width of the content area (n8n parity) and needs the whole
 * viewport height to fit the workflow graph. The version-history page
 * (`/workflows/:id/versions`) is a regular page and stays inside the
 * content container.
 */
const isFullBleed = computed(
  () =>
    /^\/workflows\/(new|[^/]+)$/.test(route.path) ||
    /^\/executions\/[^/]+$/.test(route.path),
)

onMounted(async () => {
  // Initialize auth
  authStore.initAuth()

  // Fetch node types
  try {
    await nodesStore.fetchNodeTypes()
  } catch (e) {
    console.error('Failed to fetch node types:', e)
  }

  // Connect WebSocket
  wsConnection.connect()
})
</script>

<template>
  <div class="flex h-screen overflow-hidden bg-neutral-50 dark:bg-neutral-950">
    <!-- Sidebar -->
    <Sidebar />

    <!-- Main content area -->
    <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <!-- Header -->
      <Header />

      <!-- Optional banner slot (48px) -->
      <div
        v-if="$slots.banner"
        class="flex h-12 shrink-0 items-center border-b border-neutral-200 bg-white px-6 dark:border-neutral-800 dark:bg-neutral-900"
      >
        <slot name="banner" />
      </div>

      <!-- Page content -->
      <main class="flex-1 overflow-auto">
        <div
          v-if="isFullBleed"
          class="h-full"
        >
          <RouterView />
        </div>
        <div
          v-else
          class="mx-auto w-full max-w-[1280px] px-6 py-6"
        >
          <RouterView />
        </div>
      </main>
    </div>
  </div>
</template>
