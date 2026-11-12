<script setup lang="ts">
/**
 * Templates Hub Component
 * Curated workflow templates for quick start
 */
import { ref, computed } from 'vue';
import {
  MagnifyingGlassIcon,
  RocketLaunchIcon,
  ClockIcon,
  GlobeAltIcon,
  CircleStackIcon,
  ChatBubbleLeftRightIcon,
  EnvelopeIcon,
  CpuChipIcon,
  CloudIcon,
  DocumentDuplicateIcon,
  ArrowTopRightOnSquareIcon,
} from '@heroicons/vue/24/outline';
import IconButton from '@/components/common/IconButton.vue';
import StatusBadge from '@/components/common/StatusBadge.vue';

const emit = defineEmits<{
  useTemplate: [template: Template];
  close: [];
}>();

interface Template {
  id: string;
  name: string;
  description: string;
  category: string;
  tags: string[];
  nodeCount: number;
  usageCount: number;
  author: string;
  featured: boolean;
}

// State
const searchQuery = ref('');
const selectedCategory = ref('all');

// Categories
const categories = [
  { id: 'all', name: 'All Templates', icon: DocumentDuplicateIcon },
  { id: 'trigger', name: 'Triggers', icon: RocketLaunchIcon },
  { id: 'schedule', name: 'Scheduled', icon: ClockIcon },
  { id: 'api', name: 'API & HTTP', icon: GlobeAltIcon },
  { id: 'database', name: 'Database', icon: CircleStackIcon },
  { id: 'messaging', name: 'Messaging', icon: ChatBubbleLeftRightIcon },
  { id: 'email', name: 'Email', icon: EnvelopeIcon },
  { id: 'ai', name: 'AI & ML', icon: CpuChipIcon },
  { id: 'cloud', name: 'Cloud', icon: CloudIcon },
];

// Sample templates (would be fetched from API in production)
const templates = ref<Template[]>([
  {
    id: 'http-webhook',
    name: 'HTTP Webhook Handler',
    description: 'Receive and process incoming HTTP webhooks with validation and routing.',
    category: 'trigger',
    tags: ['webhook', 'http', 'api', 'automation'],
    nodeCount: 3,
    usageCount: 12500,
    author: 'm9m',
    featured: true,
  },
  {
    id: 'scheduled-backup',
    name: 'Scheduled Database Backup',
    description: 'Automatically backup your database on a schedule and upload to cloud storage.',
    category: 'schedule',
    tags: ['backup', 'database', 'cron', 's3'],
    nodeCount: 5,
    usageCount: 8700,
    author: 'm9m',
    featured: true,
  },
  {
    id: 'slack-notifications',
    name: 'Slack Alert Pipeline',
    description: 'Send custom notifications to Slack channels based on workflow events.',
    category: 'messaging',
    tags: ['slack', 'notifications', 'alerts'],
    nodeCount: 4,
    usageCount: 15200,
    author: 'm9m',
    featured: true,
  },
  {
    id: 'email-automation',
    name: 'Email Marketing Automation',
    description: 'Automated email sequences with personalization and tracking.',
    category: 'email',
    tags: ['email', 'marketing', 'automation'],
    nodeCount: 6,
    usageCount: 9300,
    author: 'm9m',
    featured: false,
  },
  {
    id: 'ai-content',
    name: 'AI Content Generator',
    description: 'Generate content using AI with custom prompts and templates.',
    category: 'ai',
    tags: ['ai', 'gpt', 'content', 'generation'],
    nodeCount: 4,
    usageCount: 21000,
    author: 'm9m',
    featured: true,
  },
  {
    id: 'api-aggregator',
    name: 'Multi-API Data Aggregator',
    description: 'Fetch data from multiple APIs, merge, and transform for unified output.',
    category: 'api',
    tags: ['api', 'aggregation', 'transform'],
    nodeCount: 7,
    usageCount: 6800,
    author: 'm9m',
    featured: false,
  },
  {
    id: 'postgres-sync',
    name: 'PostgreSQL Data Sync',
    description: 'Sync data between PostgreSQL databases with conflict resolution.',
    category: 'database',
    tags: ['postgres', 'sync', 'database'],
    nodeCount: 5,
    usageCount: 4200,
    author: 'm9m',
    featured: false,
  },
  {
    id: 'cloud-monitor',
    name: 'Cloud Resource Monitor',
    description: 'Monitor cloud resources and trigger alerts based on thresholds.',
    category: 'cloud',
    tags: ['aws', 'monitoring', 'alerts'],
    nodeCount: 6,
    usageCount: 3900,
    author: 'm9m',
    featured: false,
  },
  {
    id: 'discord-bot',
    name: 'Discord Bot Commands',
    description: 'Handle Discord bot commands with custom responses and actions.',
    category: 'messaging',
    tags: ['discord', 'bot', 'commands'],
    nodeCount: 4,
    usageCount: 11500,
    author: 'm9m',
    featured: false,
  },
  {
    id: 'ai-chatbot',
    name: 'AI Customer Support Bot',
    description: 'AI-powered chatbot for customer support with context memory.',
    category: 'ai',
    tags: ['ai', 'chatbot', 'support', 'claude'],
    nodeCount: 8,
    usageCount: 18700,
    author: 'm9m',
    featured: true,
  },
]);

// Filtered templates
const filteredTemplates = computed(() => {
  return templates.value.filter((template) => {
    const matchesCategory =
      selectedCategory.value === 'all' || template.category === selectedCategory.value;
    const matchesSearch =
      !searchQuery.value ||
      template.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
      template.description.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
      template.tags.some((tag) => tag.toLowerCase().includes(searchQuery.value.toLowerCase()));

    return matchesCategory && matchesSearch;
  });
});

// Featured templates
const featuredTemplates = computed(() => {
  return templates.value.filter((t) => t.featured).slice(0, 4);
});

// Use template
async function useTemplate(template: Template) {
  emit('useTemplate', template);
}

// Format usage count
function formatUsageCount(count: number): string {
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1)}k`;
  }
  return count.toString();
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-[oklch(100%_0_89.88/0.8)] p-4 backdrop-blur-[2px] dark:bg-black/60">
    <div class="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-neutral-200 bg-white shadow-xl dark:border-neutral-800 dark:bg-neutral-900">
      <!-- Header -->
      <div class="border-b border-neutral-200 px-6 py-4 dark:border-neutral-800">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-xl font-semibold text-neutral-950 dark:text-neutral-50">
              Templates Hub
            </h2>
            <p class="mt-1 text-xs text-neutral-600 dark:text-neutral-400">
              Start faster with pre-built workflow templates
            </p>
          </div>
          <IconButton
            label="Close templates hub"
            @click="emit('close')"
          >
            <svg
              class="h-4 w-4"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </IconButton>
        </div>

        <!-- Search -->
        <div class="mt-4 flex items-center gap-4">
          <div class="relative flex-1">
            <MagnifyingGlassIcon class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-neutral-400" />
            <input
              v-model="searchQuery"
              type="text"
              placeholder="Search templates…"
              class="h-10 w-full rounded-lg border border-neutral-300 bg-white pl-9 pr-4 text-sm text-neutral-950 transition-colors duration-200 placeholder:text-neutral-400 focus:border-neutral-900 focus:outline-none focus:ring-2 focus:ring-neutral-900/10 dark:border-neutral-700 dark:bg-neutral-950 dark:text-neutral-100 dark:focus:border-neutral-100 dark:focus:ring-neutral-100/10"
            >
          </div>
        </div>
      </div>

      <div class="flex flex-1 overflow-hidden">
        <!-- Sidebar Categories -->
        <div class="w-48 overflow-y-auto border-r border-neutral-200 p-4 dark:border-neutral-800">
          <h3 class="mb-3 text-2xs font-medium uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            Categories
          </h3>
          <nav class="space-y-1">
            <button
              v-for="category in categories"
              :key="category.id"
              type="button"
              class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm transition-colors duration-200"
              :class="
                selectedCategory === category.id
                  ? 'bg-brand-50 font-medium text-brand-700 dark:bg-brand-950/50 dark:text-brand-300'
                  : 'text-neutral-600 hover:bg-neutral-100 dark:text-neutral-400 dark:hover:bg-neutral-800'
              "
              @click="selectedCategory = category.id"
            >
              <component
                :is="category.icon"
                class="h-4 w-4"
              />
              {{ category.name }}
            </button>
          </nav>
        </div>

        <!-- Templates Grid -->
        <div class="flex-1 overflow-y-auto p-6">
          <!-- Featured Section -->
          <div
            v-if="selectedCategory === 'all' && !searchQuery"
            class="mb-8"
          >
            <h3 class="mb-4 flex items-center gap-2 text-lg font-semibold text-neutral-950 dark:text-neutral-50">
              <RocketLaunchIcon class="h-4 w-4 text-brand-500" />
              Featured Templates
            </h3>
            <div class="grid grid-cols-2 gap-4">
              <div
                v-for="template in featuredTemplates"
                :key="template.id"
                class="cursor-pointer rounded-xl border border-brand-200 bg-brand-50/60 p-4 transition-colors duration-200 hover:border-brand-300 hover:bg-brand-50 dark:border-brand-900 dark:bg-brand-950/30 dark:hover:border-brand-800 dark:hover:bg-brand-950/50"
                @click="useTemplate(template)"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <h4 class="text-sm font-semibold text-neutral-950 dark:text-neutral-50">
                      {{ template.name }}
                    </h4>
                    <p class="mt-1 line-clamp-2 text-xs text-neutral-600 dark:text-neutral-400">
                      {{ template.description }}
                    </p>
                  </div>
                  <StatusBadge
                    status="info"
                    label="Featured"
                  />
                </div>
                <div class="mt-3 flex items-center gap-4 text-2xs text-neutral-500 dark:text-neutral-400">
                  <span>{{ template.nodeCount }} nodes</span>
                  <span>{{ formatUsageCount(template.usageCount) }} uses</span>
                </div>
              </div>
            </div>
          </div>

          <!-- All Templates -->
          <div>
            <h3 class="mb-4 text-lg font-semibold text-neutral-950 dark:text-neutral-50">
              {{ selectedCategory === 'all' ? 'All Templates' : categories.find(c => c.id === selectedCategory)?.name }}
              <span class="ml-2 text-xs font-normal text-neutral-500 dark:text-neutral-400">
                ({{ filteredTemplates.length }})
              </span>
            </h3>

            <div
              v-if="filteredTemplates.length"
              class="grid grid-cols-2 gap-4"
            >
              <div
                v-for="template in filteredTemplates"
                :key="template.id"
                class="group cursor-pointer rounded-xl border border-neutral-200 bg-white p-4 transition-colors duration-200 hover:border-brand-300 dark:border-neutral-800 dark:bg-neutral-900 dark:hover:border-brand-800"
                @click="useTemplate(template)"
              >
                <div class="flex items-start justify-between gap-3">
                  <h4 class="text-sm font-semibold text-neutral-950 transition-colors duration-200 group-hover:text-brand-700 dark:text-neutral-50 dark:group-hover:text-brand-300">
                    {{ template.name }}
                  </h4>
                  <ArrowTopRightOnSquareIcon class="h-4 w-4 shrink-0 text-neutral-400 opacity-0 transition-opacity duration-200 group-hover:opacity-100" />
                </div>
                <p class="mt-1 line-clamp-2 text-xs text-neutral-600 dark:text-neutral-400">
                  {{ template.description }}
                </p>
                <div class="mt-3 flex flex-wrap gap-1">
                  <span
                    v-for="tag in template.tags.slice(0, 3)"
                    :key="tag"
                    class="inline-flex h-5 items-center rounded-lg border border-neutral-200 bg-neutral-50 px-2 text-2xs text-neutral-600 dark:border-neutral-700 dark:bg-neutral-800 dark:text-neutral-300"
                  >
                    {{ tag }}
                  </span>
                </div>
                <div class="mt-3 flex items-center justify-between text-2xs text-neutral-500 dark:text-neutral-400">
                  <span>{{ template.nodeCount }} nodes</span>
                  <span>{{ formatUsageCount(template.usageCount) }} uses</span>
                </div>
              </div>
            </div>

            <div
              v-else
              class="px-4 py-12 text-center text-neutral-600 dark:text-neutral-400"
            >
              <DocumentDuplicateIcon class="mx-auto mb-3 h-12 w-12 opacity-50" />
              <p class="text-sm font-medium">No templates found</p>
              <p class="mt-1 text-xs">Try adjusting your search or category</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
