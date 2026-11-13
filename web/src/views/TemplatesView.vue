<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import TemplatesHub from '@/components/templates/TemplatesHub.vue';
import apiClient from '@/api/client';

const router = useRouter();
const showHub = ref(true);

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

async function handleUseTemplate(template: Template) {
  // Apply template via API and navigate to editor. The shared client
  // attaches the session's bearer token — the backend rejects
  // anonymous /api/v1 calls with 401 once auth is enabled.
  try {
    const response = await apiClient.post<{ workflowId?: string }>(
      `/templates/${template.id}/apply`
    );

    const data = response.data;
    if (data?.workflowId) {
      router.push(`/workflows/${data.workflowId}`);
    } else {
      router.push('/workflows/new');
    }
  } catch (error) {
    console.error('Failed to apply template:', error);
    router.push('/workflows/new');
  }
}

function handleClose() {
  router.push('/workflows');
}
</script>

<template>
  <div class="h-full">
    <TemplatesHub
      v-if="showHub"
      @use-template="handleUseTemplate"
      @close="handleClose"
    />
  </div>
</template>
