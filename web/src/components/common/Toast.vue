<script setup lang="ts">
/**
 * Toast Component
 * Displays toast notifications with animations and actions.
 *
 * Props / emits are unchanged — only the visual treatment was moved to
 * the m9m token layer (8px radius, shadow-md, status-tinted border).
 */
import { computed } from 'vue';
import {
  CheckCircleIcon,
  ExclamationCircleIcon,
  ExclamationTriangleIcon,
  InformationCircleIcon,
  XMarkIcon,
} from '@heroicons/vue/24/outline';
import type { Notification, NotificationType } from '@/stores/notifications';

const props = defineProps<{
  notification: Notification;
}>();

const emit = defineEmits<{
  dismiss: [id: string];
}>();

// Icon based on notification type
const icons: Record<NotificationType, typeof CheckCircleIcon> = {
  success: CheckCircleIcon,
  error: ExclamationCircleIcon,
  warning: ExclamationTriangleIcon,
  info: InformationCircleIcon,
};

const icon = computed(() => icons[props.notification.type]);

/**
 * Styles based on notification type — status family from the token
 * contract: success green-50/200/800, warning yellow-100/200/800,
 * danger red-50/200/800, info blue-50/200/800.
 */
const styles = computed(() => {
  const baseStyles = {
    success: {
      bg: 'bg-green-50 dark:bg-green-900/20',
      border: 'border-green-200 dark:border-green-800',
      icon: 'text-green-600 dark:text-green-400',
      title: 'text-green-800 dark:text-green-200',
      message: 'text-green-700 dark:text-green-300',
    },
    error: {
      bg: 'bg-red-50 dark:bg-red-900/20',
      border: 'border-red-200 dark:border-red-800',
      icon: 'text-red-600 dark:text-red-400',
      title: 'text-red-800 dark:text-red-200',
      message: 'text-red-700 dark:text-red-300',
    },
    warning: {
      bg: 'bg-yellow-100 dark:bg-yellow-900/20',
      border: 'border-yellow-200 dark:border-yellow-800',
      icon: 'text-yellow-700 dark:text-yellow-400',
      title: 'text-yellow-800 dark:text-yellow-200',
      message: 'text-yellow-800 dark:text-yellow-300',
    },
    info: {
      bg: 'bg-blue-50 dark:bg-blue-900/20',
      border: 'border-blue-200 dark:border-blue-800',
      icon: 'text-blue-600 dark:text-blue-400',
      title: 'text-blue-800 dark:text-blue-200',
      message: 'text-blue-700 dark:text-blue-300',
    },
  };

  return baseStyles[props.notification.type];
});

function handleDismiss() {
  emit('dismiss', props.notification.id);
}

function handleAction() {
  if (props.notification.action?.handler) {
    props.notification.action.handler();
  }
  handleDismiss();
}
</script>

<template>
  <div
    class="pointer-events-auto w-full max-w-sm overflow-hidden rounded-lg border bg-white shadow-md transition-all duration-200 ease-out dark:bg-neutral-900"
    :class="[styles.bg, styles.border]"
    role="status"
    aria-live="polite"
  >
    <div class="flex items-start gap-3 p-4">
      <!-- Icon -->
      <div class="flex-shrink-0 pt-0.5">
        <component
          :is="icon"
          class="h-5 w-5"
          :class="styles.icon"
          aria-hidden="true"
        />
      </div>

      <!-- Content -->
      <div class="min-w-0 flex-1">
        <p
          class="text-sm font-semibold"
          :class="styles.title"
        >
          {{ notification.title }}
        </p>
        <p
          v-if="notification.message"
          class="mt-1 text-xs leading-relaxed"
          :class="styles.message"
        >
          {{ notification.message }}
        </p>

        <!-- Action button -->
        <div
          v-if="notification.action"
          class="mt-3"
        >
          <button
            type="button"
            class="rounded-sm text-xs font-medium underline-offset-2 transition-colors hover:underline focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500"
            :class="styles.title"
            @click="handleAction"
          >
            {{ notification.action.label }}
          </button>
        </div>
      </div>

      <!-- Dismiss button -->
      <div
        v-if="notification.dismissible"
        class="flex flex-shrink-0"
      >
        <button
          type="button"
          class="inline-flex h-6 w-6 items-center justify-center rounded-sm opacity-60 transition-opacity hover:opacity-100 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-500"
          :class="styles.icon"
          @click="handleDismiss"
        >
          <span class="sr-only">Dismiss</span>
          <XMarkIcon
            class="h-4 w-4"
            aria-hidden="true"
          />
        </button>
      </div>
    </div>
  </div>
</template>
