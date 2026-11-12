<script setup lang="ts">
import { useRouter } from 'vue-router'
import { HomeIcon } from '@heroicons/vue/24/outline'
import Button from '@/components/common/Button.vue'

const router = useRouter()

const goHome = () => {
  router.push('/')
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-neutral-50 px-4 dark:bg-neutral-950">
    <div class="text-center">
      <p class="text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">
        404
      </p>
      <h1 class="mt-3 text-xl font-semibold text-neutral-950 dark:text-neutral-50">
        Page not found
      </h1>
      <p class="mx-auto mt-2 max-w-sm text-sm text-neutral-600 dark:text-neutral-400">
        The page you're looking for doesn't exist or has been moved.
      </p>
      <Button
        variant="primary"
        size="md"
        class="mt-6"
        @click="goHome"
      >
        <template #icon>
          <HomeIcon class="h-4 w-4" />
        </template>
        <span>Go to Dashboard</span>
      </Button>
    </div>
  </div>
</template>
