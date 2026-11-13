<script setup lang="ts">
/**
 * AgentAI Component
 *
 * AI-powered workflow assistance for m9m. Replaces the legacy
 * "Agent Copilot" panel. All HTTP traffic goes through /api/v1/ai/*;
 * the panel degrades gracefully when the AI runtime is not configured
 * (Settings → AI).
 */
import { ref, watch, computed } from 'vue';
import {
  SparklesIcon,
  PaperAirplaneIcon,
  XMarkIcon,
  LightBulbIcon,
  ChatBubbleLeftRightIcon,
  DocumentTextIcon,
  ChevronDownIcon,
  ChevronUpIcon,
} from '@heroicons/vue/24/outline';
import { chatWithAI, generateAIWorkflow, suggestAINodes } from '@/api/ai';

const props = defineProps<{
  workflow?: object;
  isOpen?: boolean;
  /**
   * embedded=true means the panel is rendered inside a flex column of
   * the workflow editor (the right-side slide-in panel). In that mode
   * it expands to fill the parent's height (`h-full`) instead of the
   * fixed `h-[600px]` size used when the panel floats over the page.
   * embedded=false (default) keeps the legacy floating behaviour for
   * any other mount point (Settings, future modals, etc.).
   */
  embedded?: boolean;
}>();

const emit = defineEmits<{
  close: [];
  applyWorkflow: [workflow: object];
  addNode: [node: object];
}>();

// State
const activeTab = ref<'chat' | 'generate' | 'suggest' | 'explain'>('chat');
const isExpanded = ref(true);
const isLoading = ref(false);
const chatInput = ref('');
const generateInput = ref('');

/*
 * Panel geometry — n8n parity: a right-hand column of 400px that
 * can be resized between 380px and 425px.
 */
const PANEL_MIN_WIDTH = 380;
const PANEL_MAX_WIDTH = 425;
const PANEL_DEFAULT_WIDTH = 400;

const panelWidth = ref(PANEL_DEFAULT_WIDTH);

const panelStyle = computed(() => ({
  width: `${panelWidth.value}px`,
}));

/**
 * Drags the panel's left edge. The pointer is tracked on the window
 * so the drag survives the pointer leaving the grip.
 */
function startResize(event: PointerEvent) {
  event.preventDefault();
  const startX = event.clientX;
  const startWidth = panelWidth.value;

  const onMove = (moveEvent: PointerEvent) => {
    // The grip sits on the left edge, so dragging left widens the panel.
    const next = startWidth + (startX - moveEvent.clientX);
    panelWidth.value = Math.min(Math.max(next, PANEL_MIN_WIDTH), PANEL_MAX_WIDTH);
  };

  const onUp = () => {
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
  };

  window.addEventListener('pointermove', onMove);
  window.addEventListener('pointerup', onUp);
}

// Chat messages
interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  timestamp: Date;
  actions?: Array<{ type: string; description: string; data: object }>;
}

const chatMessages = ref<ChatMessage[]>([
  {
    role: 'assistant',
    content: "Hi! I'm your AI assistant. I can help you build workflows, suggest nodes, explain your workflow, or fix errors. What would you like to do?",
    timestamp: new Date(),
  },
]);

// Suggestions
const nodeSuggestions = ref<Array<{
  type: string;
  name: string;
  description: string;
  reason: string;
  confidence: number;
}>>([]);

// Generated workflow
const generatedWorkflow = ref<object | null>(null);
const generationExplanation = ref('');

// All AI traffic goes through /api/v1/ai/* via the shared API client,
// which attaches the session's bearer token — the backend rejects
// anonymous /api/v1 calls with 401 once auth is enabled.

// Send chat message
async function sendChatMessage() {
  if (!chatInput.value.trim() || isLoading.value) return;

  const userMessage = chatInput.value.trim();
  chatInput.value = '';

  // Add user message
  chatMessages.value.push({
    role: 'user',
    content: userMessage,
    timestamp: new Date(),
  });

  isLoading.value = true;

  try {
    const data = await chatWithAI({
      messages: chatMessages.value.map((m) => ({
        role: m.role,
        content: m.content,
      })),
      currentWorkflow: props.workflow,
    });

    chatMessages.value.push({
      role: 'assistant',
      content: data.message,
      timestamp: new Date(),
      actions: data.actions,
    });
  } catch (error) {
    chatMessages.value.push({
      role: 'assistant',
      content: 'Sorry, I encountered an error. Please try again.',
      timestamp: new Date(),
    });
  } finally {
    isLoading.value = false;
  }
}

// Generate workflow
async function generateWorkflow() {
  if (!generateInput.value.trim() || isLoading.value) return;

  isLoading.value = true;

  try {
    const data = await generateAIWorkflow(generateInput.value);
    generatedWorkflow.value = data.workflow;
    generationExplanation.value = data.explanation;
  } catch (error) {
    generationExplanation.value = 'Failed to generate workflow. Please try again.';
  } finally {
    isLoading.value = false;
  }
}

// Get node suggestions
async function getSuggestions() {
  isLoading.value = true;

  try {
    const data = await suggestAINodes({
      currentWorkflow: props.workflow,
      userQuery: 'Suggest next nodes',
    });

    nodeSuggestions.value = data.suggestions || [];
  } catch (error) {
    nodeSuggestions.value = [];
  } finally {
    isLoading.value = false;
  }
}

// Apply generated workflow
function applyGeneratedWorkflow() {
  if (generatedWorkflow.value) {
    emit('applyWorkflow', generatedWorkflow.value);
  }
}

// Add suggested node
function addSuggestedNode(suggestion: typeof nodeSuggestions.value[0]) {
  emit('addNode', {
    type: suggestion.type,
    name: suggestion.name,
    parameters: {},
  });
}

// Execute action from chat
function executeAction(action: { type: string; description: string; data: object }) {
  switch (action.type) {
    case 'add_node':
      emit('addNode', action.data);
      break;
    case 'apply_workflow':
      emit('applyWorkflow', action.data);
      break;
  }
}

// Load suggestions when tab changes
watch(activeTab, (tab) => {
  if (tab === 'suggest') {
    getSuggestions();
  }
});

// Keyboard shortcut
function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    if (activeTab.value === 'chat') {
      sendChatMessage();
    } else if (activeTab.value === 'generate') {
      generateWorkflow();
    }
  }
}
</script>

<template>
  <div
    class="m9m-panel h-full border-l border-default overflow-hidden flex flex-col relative"
    :class="{ 'h-[600px]': isExpanded && !embedded, 'h-14': !isExpanded, 'h-full': isExpanded && embedded }"
    :style="panelStyle"
  >
    <!-- Resize grip — the panel edge can be dragged between 380 and 425px. -->
    <div
      v-if="embedded && isExpanded"
      class="ai-panel__grip"
      role="separator"
      aria-orientation="vertical"
      title="Drag to resize the assistant panel"
      @pointerdown="startResize"
    />

    <!-- Header -->
    <div
      class="m9m-ai-header flex items-center justify-between px-4 py-3 text-white cursor-pointer shrink-0"
      @click="isExpanded = !isExpanded"
    >
      <div class="flex items-center gap-2">
        <SparklesIcon class="w-5 h-5" />
        <span class="font-semibold">AI</span>
        <span class="text-xs bg-white/20 px-2 py-0.5 rounded-full">assistant</span>
      </div>
      <div class="flex items-center gap-2">
        <button
          @click.stop="isExpanded = !isExpanded"
          class="p-1 hover:bg-white/20 rounded"
          aria-label="Collapse the assistant"
        >
          <component :is="isExpanded ? ChevronDownIcon : ChevronUpIcon" class="w-4 h-4" />
        </button>
        <button
          @click.stop="emit('close')"
          class="p-1 hover:bg-white/20 rounded"
          aria-label="Close the assistant"
        >
          <XMarkIcon class="w-4 h-4" />
        </button>
      </div>
    </div>

    <!-- Content -->
    <div v-if="isExpanded" class="flex flex-col flex-1 min-h-0">
      <!-- Tabs -->
      <div class="flex border-b border-default shrink-0">
        <button
          v-for="tab in [
            { id: 'chat', label: 'Chat', icon: ChatBubbleLeftRightIcon },
            { id: 'generate', label: 'Generate', icon: SparklesIcon },
            { id: 'suggest', label: 'Suggest', icon: LightBulbIcon },
            { id: 'explain', label: 'Explain', icon: DocumentTextIcon },
          ]"
          :key="tab.id"
          @click="activeTab = tab.id as typeof activeTab"
          class="flex-1 flex items-center justify-center gap-1.5 py-2.5 text-sm font-medium transition-colors"
          :class="
            activeTab === tab.id
              ? 'm9m-tab-active'
              : 'text-tertiary hover:text-secondary'
          "
        >
          <component :is="tab.icon" class="w-4 h-4" />
          {{ tab.label }}
        </button>
      </div>

      <!-- Tab Content -->
      <div class="flex-1 overflow-hidden min-h-0">
        <!-- Chat Tab -->
        <div v-if="activeTab === 'chat'" class="flex flex-col h-full">
          <!-- Messages -->
          <div class="flex-1 overflow-y-auto p-4 space-y-4">
            <div
              v-for="(message, index) in chatMessages"
              :key="index"
              class="flex"
              :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
            >
              <div
                class="max-w-[85%] rounded-lg px-3 py-2 text-sm"
                :class="
                  message.role === 'user'
                    ? 'm9m-bubble m9m-bubble--user text-white'
                    : 'm9m-bubble m9m-bubble--assistant text-primary'
                "
              >
                <p class="whitespace-pre-wrap">{{ message.content }}</p>

                <!-- Actions -->
                <div v-if="message.actions?.length" class="mt-2 space-y-1">
                  <button
                    v-for="(action, i) in message.actions"
                    :key="i"
                    @click="executeAction(action)"
                    class="m9m-action-chip w-full text-left px-2 py-1 rounded text-xs"
                  >
                    {{ action.description }}
                  </button>
                </div>
              </div>
            </div>

            <!-- Loading indicator -->
            <div v-if="isLoading" class="flex justify-start">
              <div class="bg-subtle rounded-lg px-4 py-2">
                <div class="flex items-center gap-2 text-sm text-tertiary">
                  <div class="animate-pulse">Thinking...</div>
                </div>
              </div>
            </div>
          </div>

          <!-- Input -->
          <div class="p-3 border-t border-default shrink-0">
            <div class="flex items-center gap-2">
              <input
                v-model="chatInput"
                @keydown="handleKeydown"
                type="text"
                placeholder="Ask me anything about workflows..."
                class="input flex-1 h-10 px-3"
              />
              <button
                @click="sendChatMessage"
                :disabled="isLoading || !chatInput.trim()"
                class="m9m-btn-primary h-10 w-10 inline-flex items-center justify-center rounded-lg text-white disabled:opacity-50 disabled:cursor-not-allowed"
              >
                <PaperAirplaneIcon class="w-4 h-4" />
              </button>
            </div>
          </div>
        </div>

        <!-- Generate Tab -->
        <div v-else-if="activeTab === 'generate'" class="flex flex-col h-full p-4">
          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-secondary mb-1">
                Describe your workflow
              </label>
              <textarea
                v-model="generateInput"
                @keydown="handleKeydown"
                rows="4"
                placeholder="e.g., When I receive a webhook, send the data to Slack and save it to a database..."
                class="input resize-none"
              ></textarea>
            </div>

            <button
              @click="generateWorkflow"
              :disabled="isLoading || !generateInput.trim()"
              class="m9m-btn-primary w-full h-10 rounded-lg text-white font-medium disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
            >
              <SparklesIcon class="w-4 h-4" />
              {{ isLoading ? 'Generating...' : 'Generate Workflow' }}
            </button>

            <!-- Generated Result -->
            <div v-if="generatedWorkflow" class="mt-4 p-3 bg-subtle rounded-lg">
              <p class="text-sm text-secondary mb-3">
                {{ generationExplanation }}
              </p>
              <button
                @click="applyGeneratedWorkflow"
                class="w-full h-10 rounded-lg bg-green-600 text-white font-medium hover:bg-green-700 flex items-center justify-center gap-2"
              >
                Apply Workflow
              </button>
            </div>
          </div>
        </div>

        <!-- Suggest Tab -->
        <div v-else-if="activeTab === 'suggest'" class="h-full overflow-y-auto p-4">
          <div v-if="isLoading" class="flex items-center justify-center h-32">
            <div class="text-tertiary">Loading suggestions...</div>
          </div>

          <div v-else-if="nodeSuggestions.length" class="space-y-3">
            <div
              v-for="(suggestion, index) in nodeSuggestions"
              :key="index"
              class="p-3 bg-subtle rounded-lg hover:bg-hover cursor-pointer transition-colors"
              @click="addSuggestedNode(suggestion)"
            >
              <div class="flex items-start justify-between">
                <div>
                  <h4 class="font-medium text-primary">
                    {{ suggestion.name }}
                  </h4>
                  <p class="text-sm text-tertiary mt-0.5">
                    {{ suggestion.description }}
                  </p>
                  <p class="text-xs text-brand-600 dark:text-brand-400 mt-1">
                    {{ suggestion.reason }}
                  </p>
                </div>
                <span
                  class="text-xs font-medium px-2 py-0.5 rounded-full"
                  :class="
                    suggestion.confidence > 0.8
                      ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                      : 'bg-yellow-100 text-yellow-700 dark:bg-yellow-500/15 dark:text-yellow-300'
                  "
                >
                  {{ Math.round(suggestion.confidence * 100) }}%
                </span>
              </div>
            </div>
          </div>

          <div v-else class="text-center py-8 text-tertiary">
            <LightBulbIcon class="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>No suggestions available</p>
            <p class="text-sm">Build more of your workflow to get suggestions</p>
          </div>
        </div>

        <!-- Explain Tab -->
        <div v-else-if="activeTab === 'explain'" class="h-full overflow-y-auto p-4">
          <div class="text-center py-8 text-tertiary">
            <DocumentTextIcon class="w-12 h-12 mx-auto mb-3 opacity-50" />
            <p>Select a workflow to explain</p>
            <p class="text-sm mt-1">The AI assistant will analyze and explain your workflow</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/*
 * The token layer publishes `--background--surface`, `--background--brand`
 * and `--background--subtle`; the fallbacks keep the panel correct
 * until (or if) it lands.
 */
.m9m-panel {
  background: var(--background--surface, #ffffff);
}

:global(.dark) .m9m-panel {
  background: var(--background--surface, oklch(26.86% 0 89.88));
}

/* The assistant bubble sits on the subtle surface. */
.m9m-bubble--assistant {
  background: var(--background--subtle, oklch(97.02% 0 89.88));
}

:global(.dark) .m9m-bubble--assistant {
  background: var(--background--subtle, oklch(31.71% 0 89.88));
}

/* The user bubble is filled with the brand colour. */
.m9m-bubble--user {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
}

/* The active tab carries the brand underline and label colour. */
.m9m-tab-active {
  color: var(--background--brand--hover, oklch(68.08% 0.148 5.02));
  border-bottom: 2px solid var(--background--brand, oklch(78.07% 0.136 5.02));
}

/* An action chip inside an assistant bubble. */
.m9m-action-chip {
  background: var(--background--brand--tint--dark, oklch(31.71% 0.136 5.02 / 0.15));
  color: var(--background--brand--tint--foreground--dark, oklch(85.55% 0.105 5.02));
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.m9m-action-chip:hover {
  background: var(--background--brand--tint--dark-hover, oklch(31.71% 0.136 5.02 / 0.25));
}

.m9m-btn-primary {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
  transition: background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

.m9m-btn-primary:hover:not(:disabled) {
  background: var(--background--brand--hover, oklch(68.08% 0.148 5.02));
}

.m9m-btn-primary:active:not(:disabled) {
  background: var(--background--brand--active, oklch(59.09% 0.1611 5.09));
}

/*
 * The assistant header carries the published assistant gradient. The
 * token layer ships a light and a dark variant; the fallback is the
 * n8n assistant gradient.
 */
.m9m-ai-header {
  background: var(
    --assistant--color--highlight-gradient,
    linear-gradient(105deg, #5b60e8 0%, #aa7bec 50%, #ec7b8e 100%)
  );
}

/*
 * The resize grip on the panel's left edge. The drag itself is
 * handled in the component script (pointer events on the window).
 */
.ai-panel__grip {
  position: absolute;
  top: 0;
  left: 0;
  width: 6px;
  height: 100%;
  cursor: col-resize;
  z-index: 2;
}

.ai-panel__grip:hover {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
  opacity: 0.15;
}

@media (prefers-reduced-motion: reduce) {
  .m9m-btn-primary {
    transition-duration: 1ms;
  }
}
</style>
