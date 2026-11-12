<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { BoltIcon } from '@heroicons/vue/24/outline'
import { useAuthStore, useThemeStore } from '@/stores'
import Button from '@/components/common/Button.vue'
import Input from '@/components/common/Input.vue'

const router = useRouter()
const authStore = useAuthStore()
const themeStore = useThemeStore()

const isLogin = ref(true)
const loading = ref(false)
const error = ref('')

const form = ref({
  email: '',
  password: '',
  firstName: '',
  lastName: '',
})

const submit = async () => {
  loading.value = true
  error.value = ''

  try {
    if (isLogin.value) {
      await authStore.login({
        email: form.value.email,
        password: form.value.password,
      })
    } else {
      await authStore.register({
        email: form.value.email,
        password: form.value.password,
        firstName: form.value.firstName,
        lastName: form.value.lastName,
      })
    }
    router.push('/')
  } catch (e) {
    error.value = e instanceof Error ? e.message : 'Authentication failed'
  } finally {
    loading.value = false
  }
}

const toggleMode = () => {
  isLogin.value = !isLogin.value
  error.value = ''
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center bg-neutral-50 px-4 py-10 dark:bg-neutral-950">
    <div class="w-full max-w-md">
      <!-- Logo -->
      <div class="mb-8 text-center">
        <div class="inline-flex items-center gap-3">
          <div class="flex h-11 w-11 items-center justify-center rounded-xl bg-brand-500">
            <BoltIcon class="h-6 w-6 text-white" />
          </div>
          <span class="text-2xl font-semibold tracking-tight text-neutral-950 dark:text-neutral-50">m9m</span>
        </div>
        <p class="mt-2 text-sm text-neutral-600 dark:text-neutral-400">
          Agent-Native Workflow Automation
        </p>
      </div>

      <!-- Card -->
      <div class="rounded-xl border border-neutral-200 bg-white p-6 shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
        <h2 class="mb-6 text-center text-xl font-semibold text-neutral-950 dark:text-neutral-50">
          {{ isLogin ? 'Sign in to your account' : 'Create an account' }}
        </h2>

        <!-- Error Message -->
        <div
          v-if="error"
          class="mb-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/50 dark:text-red-300"
          role="alert"
        >
          {{ error }}
        </div>

        <form
          class="space-y-4"
          @submit.prevent="submit"
        >
          <!-- Name fields (register only) -->
          <div
            v-if="!isLogin"
            class="grid grid-cols-2 gap-4"
          >
            <Input
              v-model="form.firstName"
              type="text"
              label="First name"
              placeholder="John"
            />
            <Input
              v-model="form.lastName"
              type="text"
              label="Last name"
              placeholder="Doe"
            />
          </div>

          <Input
            v-model="form.email"
            type="email"
            label="Email"
            placeholder="you@example.com"
            required
          />

          <Input
            v-model="form.password"
            type="password"
            label="Password"
            placeholder="••••••••"
            required
          />

          <Button
            type="submit"
            variant="primary"
            size="md"
            class="w-full"
            :loading="loading"
            :disabled="loading"
          >
            {{ loading ? 'Please wait...' : (isLogin ? 'Sign in' : 'Create account') }}
          </Button>
        </form>

        <div class="mt-6 text-center">
          <button
            type="button"
            class="rounded-sm text-sm font-medium text-brand-600 transition-colors hover:text-brand-700 hover:underline focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500 dark:text-brand-400 dark:hover:text-brand-300"
            @click="toggleMode"
          >
            {{ isLogin ? "Don't have an account? Sign up" : 'Already have an account? Sign in' }}
          </button>
        </div>
      </div>

      <!-- Theme Toggle -->
      <div class="mt-6 text-center">
        <button
          type="button"
          class="rounded-sm text-sm text-neutral-500 transition-colors hover:text-neutral-800 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-neutral-950 dark:text-neutral-400 dark:hover:text-neutral-200 dark:focus-visible:outline-neutral-100"
          @click="themeStore.toggleTheme"
        >
          {{ themeStore.isDark ? 'Switch to light mode' : 'Switch to dark mode' }}
        </button>
      </div>
    </div>
  </div>
</template>
