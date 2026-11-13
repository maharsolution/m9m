<script lang="ts">
/**
 * Node icon lookup.
 *
 * n8n gives every node type its own glyph. m9m's backend ships the
 * Font-Awesome style identifier (`icon: "fa:globe"`), but the web app
 * bundles no icon font, so the identifier is mapped onto the
 * Heroicons set here. The glyph is monochrome: colour on a node card
 * belongs to the *state* (border), never to the decoration.
 *
 * The map is exported so the node creator shows the very same glyph
 * for a node type as the canvas card does.
 */
import type { Component } from 'vue'
import {
  AcademicCapIcon,
  ArrowPathIcon,
  ArrowsPointingInIcon,
  ArrowsRightLeftIcon,
  BanknotesIcon,
  BoltIcon,
  BriefcaseIcon,
  ChartBarIcon,
  ChatBubbleLeftEllipsisIcon,
  ChatBubbleLeftRightIcon,
  CircleStackIcon,
  ClockIcon,
  CloudArrowDownIcon,
  CloudIcon,
  CodeBracketIcon,
  CommandLineIcon,
  CpuChipIcon,
  CreditCardIcon,
  CubeIcon,
  CurrencyDollarIcon,
  DocumentTextIcon,
  EnvelopeIcon,
  FingerPrintIcon,
  FunnelIcon,
  GlobeAltIcon,
  LinkIcon,
  ListBulletIcon,
  MegaphoneIcon,
  PaperAirplaneIcon,
  PencilSquareIcon,
  PhoneIcon,
  PlayIcon,
  PuzzlePieceIcon,
  RectangleStackIcon,
  ServerStackIcon,
  ShareIcon,
  ShoppingBagIcon,
  SignalIcon,
  SparklesIcon,
  TableCellsIcon,
  TicketIcon,
  TruckIcon,
  WrenchScrewdriverIcon,
} from '@heroicons/vue/24/outline'

/** Glyph per node type (key: the type name without its package prefix). */
const NODE_ICON_BY_TYPE: Record<string, Component> = {
  // triggers
  webhook: LinkIcon,
  webhooktrigger: LinkIcon,
  cron: ClockIcon,
  schedule: ClockIcon,
  schedules: ClockIcon,
  scheduletrigger: ClockIcon,
  formtrigger: DocumentTextIcon,
  manualtrigger: PlayIcon,
  start: PlayIcon,
  intervtrigger: ClockIcon,
  // core / flow
  set: PencilSquareIcon,
  editfields: PencilSquareIcon,
  if: ArrowsRightLeftIcon,
  switch: ArrowsRightLeftIcon,
  merge: ArrowsPointingInIcon,
  filter: FunnelIcon,
  splitinbatches: ArrowsRightLeftIcon,
  noop: CommandLineIcon,
  wait: ClockIcon,
  looper: ArrowPathIcon,
  stopanderror: BoltIcon,
  executeworkflow: CubeIcon,
  executecommand: CommandLineIcon,
  // code
  code: CodeBracketIcon,
  function: CommandLineIcon,
  functionitem: CommandLineIcon,
  html: CodeBracketIcon,
  xml: CodeBracketIcon,
  // http / cloud
  httprequest: GlobeAltIcon,
  httpbin: GlobeAltIcon,
  graphql: GlobeAltIcon,
  ftp: CloudArrowDownIcon,
  ssh: ServerStackIcon,
  awslambda: CloudIcon,
  awss3: CloudArrowDownIcon,
  googledrive: CloudIcon,
  dropbox: CloudArrowDownIcon,
  // databases
  postgres: CircleStackIcon,
  mysql: CircleStackIcon,
  mongodb: CircleStackIcon,
  redis: CircleStackIcon,
  sqlite: CircleStackIcon,
  elasticsearch: ServerStackIcon,
  qdrant: CircleStackIcon,
  pinecone: CircleStackIcon,
  supabase: CircleStackIcon,
  googlebigquery: CircleStackIcon,
  // communication
  slack: ChatBubbleLeftEllipsisIcon,
  discord: ChatBubbleLeftEllipsisIcon,
  telegram: PaperAirplaneIcon,
  email: EnvelopeIcon,
  sendemail: EnvelopeIcon,
  gmail: EnvelopeIcon,
  microsoftoutlook: EnvelopeIcon,
  twilio: PhoneIcon,
  mailcheck: EnvelopeIcon,
  discourse: ChatBubbleLeftRightIcon,
  mattermost: ChatBubbleLeftRightIcon,
  rocketchat: ChatBubbleLeftRightIcon,
  zulip: ChatBubbleLeftRightIcon,
  // productivity
  googlesheets: TableCellsIcon,
  airtable: TableCellsIcon,
  excel: TableCellsIcon,
  baserow: TableCellsIcon,
  notion: DocumentTextIcon,
  trello: RectangleStackIcon,
  jira: TicketIcon,
  hubspot: ChartBarIcon,
  pipedrive: CurrencyDollarIcon,
  stripe: CreditCardIcon,
  shopify: ShoppingBagIcon,
  woocommerce: ShoppingBagIcon,
  odoo: CubeIcon,
  sap: BriefcaseIcon,
  erp: BriefcaseIcon,
  banner: MegaphoneIcon,
  customerio: MegaphoneIcon,
  // ai
  openai: SparklesIcon,
  anthropic: SparklesIcon,
  googlegemini: SparklesIcon,
  deepseek: SparklesIcon,
  perplexity: SparklesIcon,
  groq: SparklesIcon,
  lmstudio: CpuChipIcon,
  ollama: CpuChipIcon,
  huggingfacemodel: CpuChipIcon,
  langchain: PuzzlePieceIcon,
  n8n: PuzzlePieceIcon,
  // misc
  stickynote: PencilSquareIcon,
  rss: SignalIcon,
  keenstack: SignalIcon,
  kafka: CloudArrowDownIcon,
  amqp: CloudArrowDownIcon,
  rabbitmq: CloudArrowDownIcon,
  sqs: CloudArrowDownIcon,
  coinbase: BanknotesIcon,
  binance: BanknotesIcon,
  bybit: BanknotesIcon,
  logistic: TruckIcon,
  scholarly: AcademicCapIcon,
  gitlab: CodeBracketIcon,
  github: CodeBracketIcon,
  crypto: FingerPrintIcon,
  compression: ServerStackIcon,
  summarizer: DocumentTextIcon,
  textclassifier: DocumentTextIcon,
  listbullet: ListBulletIcon,
}

/** Glyph per node category — the fallback for unknown node types. */
const NODE_ICON_BY_CATEGORY: Record<string, Component> = {
  trigger: BoltIcon,
  action: PlayIcon,
  transform: ArrowsRightLeftIcon,
  flow: ShareIcon,
  core: CubeIcon,
  data: CircleStackIcon,
  communication: ChatBubbleLeftRightIcon,
  utility: WrenchScrewdriverIcon,
}

/**
 * Resolves the icon of a node type. The lookup uses the bare type
 * name (the part after the package prefix), lower-cased, so
 * `n8n-nodes-base.httpRequest` and `httpRequest` resolve the same.
 */
export function nodeIconFor(nodeType: string, category?: string): Component {
  const bare = String(nodeType ?? '')
    .split('.')
    .pop()!
    .toLowerCase()
  return (
    NODE_ICON_BY_TYPE[bare] ??
    NODE_ICON_BY_CATEGORY[category ?? ''] ??
    NODE_ICON_BY_CATEGORY['action']
  )
}
</script>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import '@vue-flow/node-resizer/dist/style.css'
import { XMarkIcon, KeyIcon } from '@heroicons/vue/24/outline'
import { useWorkflowEditorStore } from '@/stores'
import {
  STICKY_NOTE_COLORS,
  STICKY_NOTE_DEFAULT_HEIGHT,
  STICKY_NOTE_DEFAULT_WIDTH,
  STICKY_NOTE_TYPE,
  stickyNoteBorder,
  stickyNoteColor,
  stickyNoteTextColor,
} from '@/types'
import type { NodeCategory } from '@/types'

/**
 * The execution state of a node, painted as the card border (n8n
 * parity). Nothing in the editor store feeds this yet — the editor
 * has no per-node status — but the contract is in place so a future
 * live-execution overlay only has to set `data.status`.
 */
type NodeStatus = 'success' | 'error' | 'warning' | 'running' | 'waiting'

interface Props {
  id: string
  data: {
    label: string
    nodeType: string
    category: NodeCategory
    parameters?: Record<string, unknown>
    credentials?: Record<string, { id: string; name: string }>
    /** Optional execution state, painted as the card border. */
    status?: NodeStatus
    // 'compact' (default) is the editor-friendly node card.
    // 'comfortable' widens the card and grows the icon /
    // label for screens where the user reads the canvas (e.g.
    // the execution detail view), making each node easier to
    // click. The Canvas (editor / execution) sets this when it
    // builds the Vue Flow nodes — see `buildFlowNodes` /
    // ExecutionDetail.vue.
    density?: 'compact' | 'comfortable'
  }
  selected?: boolean
}

const props = defineProps<Props>()
const editorStore = useWorkflowEditorStore()

const isSticky = computed(() => props.data.nodeType === STICKY_NOTE_TYPE)
const isComfortable = computed(() => props.data.density === 'comfortable')

/**
 * The card never paints the category as a stripe — n8n nodes have no
 * stripe. The category only decides the fallback glyph.
 */
const iconComponent = computed(() => nodeIconFor(props.data.nodeType, props.data.category))

/** The trigger variant gets the n8n "D" radius (36px on the left). */
const isTrigger = computed(() => props.data.category === 'trigger')

const nodeStatus = computed(() => props.data.status ?? null)

const nodeTypeName = computed(() => {
  const type = props.data.nodeType
  // Extract display name from type like "n8n-nodes-base.httpRequest"
  const parts = type.split('.')
  return parts[parts.length - 1]
    .replace(/([A-Z])/g, ' $1')
    .replace(/^./, (str) => str.toUpperCase())
    .trim()
})

const credentialCount = computed(() => {
  return props.data.credentials
    ? Object.keys(props.data.credentials).length
    : 0
})

/**
 * "Notes in flow" — the node's own note, rendered on the canvas under
 * the card.
 *
 * The backend publishes two node parameters for this (see
 * `internal/nodes/nodeprops/nodeprops.go` → `Settings()`):
 * `notes` ("Optional notes shown next to the node on the canvas") and
 * `notesInFlow` ("If enabled, the node's notes render on the canvas
 * directly below the node"). n8n does the same — its
 * `getNodeSubtitle()` returns `data.notes` when `notesInFlow` is set.
 *
 * The note sits BELOW the card, on the canvas (n8n's `.description`
 * block is `position: absolute; top: 100%`), so its colour has to
 * clear 4.5:1 against the canvas background. `--text-color--subtle`
 * does, in both themes, and it clears the node surface too.
 */
const notesInFlow = computed(() => {
  const notes = props.data.parameters?.notes
  const enabled = props.data.parameters?.notesInFlow
  if (typeof notes !== 'string' || !notes.trim()) return ''
  if (enabled !== true && enabled !== 'true') return ''
  return notes
})

const deleteNode = (event: MouseEvent) => {
  event.stopPropagation()
  event.preventDefault()
  editorStore.removeNode(props.id)
}

// --------------------------------------------------------------------
// Sticky note
// --------------------------------------------------------------------

const stickyContent = computed(() => {
  const content = props.data.parameters?.content
  return typeof content === 'string' ? content : ''
})

const stickyBackground = computed(() =>
  stickyNoteColor(props.data.parameters?.color)
)

const stickyBorder = computed(() =>
  stickyNoteBorder(props.data.parameters?.color)
)

/**
 * The note body colour. Palette notes follow the theme (Graphite text
 * on the pale light palette, Neutral text on the deep dark palette);
 * a free-form hex note picks its text colour from its own luminance,
 * the way n8n's `getContrastTextColor` does.
 */
const stickyText = computed(() =>
  stickyNoteTextColor(props.data.parameters?.color)
)

const stickyWidth = computed(() => {
  const width = Number(props.data.parameters?.width)
  return Number.isFinite(width) && width > 0 ? width : STICKY_NOTE_DEFAULT_WIDTH
})

const stickyHeight = computed(() => {
  const height = Number(props.data.parameters?.height)
  return Number.isFinite(height) && height > 0 ? height : STICKY_NOTE_DEFAULT_HEIGHT
})

const editing = ref(false)
const draft = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)

const startEditing = () => {
  draft.value = stickyContent.value
  editing.value = true
  nextTick(() => {
    textareaRef.value?.focus()
    textareaRef.value?.select()
  })
}

const commitEditing = () => {
  if (!editing.value) {
    return
  }
  editing.value = false
  if (draft.value !== stickyContent.value) {
    editorStore.updateStickyNote(props.id, { content: draft.value })
  }
}

const cancelEditing = () => {
  editing.value = false
}

const setStickyColor = (index: number) => {
  editorStore.updateStickyNote(props.id, { color: index })
}

/**
 * Keeps the note's geometry in the workflow document. n8n stores the
 * sticky-note width / height inside `node.parameters`, so the note
 * survives an export / import round-trip unchanged.
 */
const onStickyResize = (payload: { params: { width: number; height: number } }) => {
  editorStore.updateStickyNote(props.id, {
    width: Math.round(payload.params.width),
    height: Math.round(payload.params.height),
  })
}
</script>

<template>
  <!-- Sticky note. n8n's `n8n-nodes-base.stickyNote` — content, colour
       and geometry live in `node.parameters`, exactly like upstream
       n8n, so the workflow document stays n8n-compatible. -->
  <div
    v-if="isSticky"
    class="sticky-note"
    :class="selected ? 'selected' : ''"
    :style="{
      backgroundColor: stickyBackground,
      borderColor: stickyBorder,
      color: stickyText,
      width: `${stickyWidth}px`,
      height: `${stickyHeight}px`,
    }"
  >
    <NodeResizer
      :is-visible="selected"
      :min-width="120"
      :min-height="80"
      color="#ec7b8e"
      @resize="onStickyResize"
    />

    <!-- Delete affordance -->
    <button
      @click="deleteNode"
      title="Delete sticky note"
      aria-label="Delete sticky note"
      class="node-delete-btn"
    >
      <XMarkIcon class="w-3.5 h-3.5" />
    </button>

    <!-- Colour swatches (n8n offers seven). -->
    <div
      v-if="selected || editing"
      class="sticky-note__colors"
      @mousedown.stop
    >
      <button
        v-for="color in STICKY_NOTE_COLORS"
        :key="color.index"
        type="button"
        class="sticky-note__swatch"
        :class="props.data.parameters?.color === color.index ? 'selected' : ''"
        :style="{ backgroundColor: color.background, borderColor: color.borderColor }"
        :title="color.name"
        @click.stop="setStickyColor(color.index)"
      />
    </div>

    <textarea
      v-if="editing"
      ref="textareaRef"
      v-model="draft"
      class="sticky-note__editor"
      placeholder="Write a note..."
      @mousedown.stop
      @click.stop
      @dblclick.stop
      @keydown.esc.stop.prevent="cancelEditing"
      @keydown.ctrl.enter.stop.prevent="commitEditing"
      @keydown.meta.enter.stop.prevent="commitEditing"
      @blur="commitEditing"
    />
    <div
      v-else
      class="sticky-note__content"
      :title="'Double click to edit'"
      @dblclick.stop="startEditing"
    >
      {{ stickyContent }}
    </div>
  </div>

  <!-- Regular node. 96 × 96 footprint, 16px radius, 1.5px hairline
       border, no resting shadow — n8n's canvas node. -->
  <div
    v-else
    class="workflow-node"
    :class="[
      isComfortable ? 'workflow-node--comfortable' : 'workflow-node--compact',
      isTrigger ? 'workflow-node--trigger' : '',
      nodeStatus ? `workflow-node--${nodeStatus}` : '',
      selected ? 'selected' : '',
    ]"
  >
    <!-- Input Handle. The :id binding (not the static `id`
         attribute) is required because Vue Flow's <Handle> declares
         `id` as a prop. Using `id="input-0"` lets Vue's attribute
         inheritance push it onto the DOM root (which is just an
         empty div, so the prop never reaches the component),
         resulting in Vue Flow rendering edges from the node center
         because both handles have null id. The :id binding makes
         it an explicit prop binding and forces the id through. -->
    <Handle
      v-if="data.category !== 'trigger'"
      :id="'input-0'"
      type="target"
      :position="Position.Left"
      :connectable="true"
      :is-connectable="true"
      class="connection-handle connection-handle--input"
    >
      <!-- The visible 16px indicator. The <Handle> itself is a 40px
           box, so the drop target stays generous without the dot
           dominating the card. -->
      <span class="handle-dot" aria-hidden="true" />
    </Handle>

    <!-- Delete affordance (top-right, hover-reveal) -->
    <button
      @click="deleteNode"
      title="Delete node"
      class="node-delete-btn"
      aria-label="Delete node"
    >
      <XMarkIcon class="w-3.5 h-3.5" />
    </button>

    <!-- Node Content. The icon sits to the LEFT of a single-line
         title; both scale with the density so the comfortable card
         (execution view) stays readable. -->
    <div class="node-body">
      <span class="node-icon" aria-hidden="true">
        <component :is="iconComponent" class="node-icon__glyph" />
      </span>

      <div class="node-text">
        <h4 class="node-title" :title="data.label">
          {{ data.label }}
        </h4>
        <p v-if="isComfortable" class="node-subtitle">
          {{ nodeTypeName }}
        </p>
      </div>
    </div>

    <!-- Credential badge. n8n marks a node with a key glyph when it
         has credentials bound. -->
    <span v-if="credentialCount > 0" class="node-credential" :title="`${credentialCount} credential${credentialCount > 1 ? 's' : ''}`">
      <KeyIcon class="w-3 h-3" />
      <span class="sr-only">{{ credentialCount }} credentials</span>
    </span>

    <!-- Parameters preview (comfortable density only — the compact
         canvas card stays a clean 96px tile). -->
    <div
      v-if="isComfortable && data.parameters && Object.keys(data.parameters).length > 0"
      class="node-parameters"
    >
      <div
        v-for="(value, key) in data.parameters"
        :key="key"
        class="node-parameter"
      >
        <span class="node-parameter__key">{{ key }}:</span>
        {{ typeof value === 'object' ? '...' : value }}
      </div>
    </div>

    <!-- Output Handle. See note on the input handle above re:
         why this uses :id instead of static `id`. -->
    <Handle
      :id="'output-0'"
      type="source"
      :position="Position.Right"
      :connectable="true"
      :is-connectable="true"
      class="connection-handle connection-handle--output"
    >
      <span class="handle-dot" aria-hidden="true" />
    </Handle>

    <!-- "Notes in flow" — the node's own note, painted on the canvas
         directly below the card (n8n parity). -->
    <p
      v-if="notesInFlow"
      class="node-notes"
      :title="notesInFlow"
    >
      {{ notesInFlow }}
    </p>
  </div>
</template>

<style scoped>
/*
 * Canvas node — n8n parity.
 *
 *  * 96 × 96 footprint (the title may widen the card so a node name
 *    stays legible), 16px radius, 1.5px hairline border.
 *  * No resting shadow: an in-layout surface gets a border, never a
 *    shadow. Shadows are reserved for overlays.
 *  * No category stripe and no coloured icon badge — the icon is
 *    monochrome, the colour belongs to the state.
 */
.workflow-node {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  min-width: 96px;
  min-height: 96px;
  max-width: 184px;
  border-radius: 16px;
  border: 1.5px solid var(--border-color, oklch(0% 0 0 / 0.1));
  background: var(--node--color--background, #ffffff);
  color: var(--text-color, oklch(20.46% 0 89.88));
  /* Handle hit areas must not be clipped by the rounded corners. */
  overflow: visible;
  /*
   * No resting shadow — an in-layout surface gets a border, never a
   * shadow. The declaration also neutralises the legacy
   * `.workflow-node` component class (shadow-md / hover:shadow-lg)
   * that still lives in main.css.
   */
  box-shadow: none;
  transition:
    border-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1),
    background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

/* Hover never raises the card — the border carries the state. */
.workflow-node:hover {
  box-shadow: none;
}

:global(.dark) .workflow-node {
  border-color: var(--border-color, oklch(100% 0 89.88 / 0.15));
  background: var(--node--color--background, hsl(0, 0%, 17%));
  color: var(--text-color, oklch(97.02% 0 89.88));
}

/* n8n's trigger variant: the left corners are rounded deeper. */
.workflow-node--trigger {
  border-radius: 36px 16px 16px 36px;
}

.workflow-node--comfortable {
  min-width: 280px;
  max-width: 340px;
  min-height: 110px;
  padding: 12px;
  flex-wrap: wrap;
}

.node-body {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1 1 auto;
}

.node-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  flex-shrink: 0;
}

.node-icon__glyph {
  width: 32px;
  height: 32px;
  color: var(--text-color, oklch(20.46% 0 89.88));
  opacity: 0.85;
}

:global(.dark) .node-icon__glyph {
  color: var(--text-color, oklch(97.02% 0 89.88));
}

.workflow-node--comfortable .node-icon {
  width: 40px;
  height: 40px;
}

.workflow-node--comfortable .node-icon__glyph {
  width: 40px;
  height: 40px;
}

.node-text {
  min-width: 0;
  flex: 1 1 auto;
}

.node-title {
  font-size: 13px;
  line-height: 1.25;
  font-weight: 600;
  letter-spacing: -0.007em;
  color: inherit;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.workflow-node--comfortable .node-title {
  font-size: 14px;
}

.node-subtitle {
  margin-top: 2px;
  font-size: 12px;
  line-height: 1.25;
  color: var(--text-color--subtle, oklch(55.55% 0 89.88));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-credential {
  position: absolute;
  left: 8px;
  bottom: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 9999px;
  color: var(--text-color--subtle, oklch(55.55% 0 89.88));
}

.node-parameters {
  flex: 1 0 100%;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border-color--subtle, oklch(0% 0 0 / 0.05));
  font-size: 12px;
  color: var(--text-color--subtle, oklch(55.55% 0 89.88));
}

.node-parameter {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-parameter__key {
  font-weight: 500;
}

/*
 * "Notes in flow" — the node's own note, painted on the canvas
 * directly below the card (n8n's `.description` block sits at
 * `top: 100%`). The note lives on the canvas, not on the card, so it
 * uses the canvas-facing body-text token: `--text-color--subtle`
 * clears 4.5:1 against the canvas (8.92:1 light / 12.70:1 dark) and
 * against the node surface as well.
 *
 * `pointer-events: none` keeps the note out of the drag / selection
 * hit-testing, so no existing canvas interaction changes.
 */
.node-notes {
  position: absolute;
  top: 100%;
  left: 0;
  width: 100%;
  margin: 4px 0 0;
  padding: 0 2px;
  font-size: 12px;
  line-height: 1.35;
  color: var(--text-color--subtle, oklch(38.67% 0 89.88));
  text-align: left;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  pointer-events: none;
  z-index: 1;
}

/*
 * Selection is a ring, not a border change — one cue per state.
 * The ring scales with the canvas zoom so it stays 6 screen pixels.
 */
.workflow-node.selected {
  box-shadow:
    0 0 0 calc(6px * var(--canvas-zoom-compensation-factor, 1)) hsla(220, 47%, 30%, 0.1);
}

/*
 * Status is encoded in the border: 2px, one hue per state.
 */
.workflow-node--success {
  border-width: 2px;
  border-color: var(--color--success, oklch(63.78% 0.155 155.83));
}

.workflow-node--error {
  border-width: 2px;
  border-color: var(--color--danger, oklch(63.78% 0.2373 25.44));
}

.workflow-node--warning {
  border-width: 2px;
  border-color: var(--color--warning, oklch(79.21% 0.162 76.95));
}

.workflow-node--running,
.workflow-node--waiting {
  border-color: transparent;
  background:
    linear-gradient(var(--node--color--background, #ffffff), var(--node--color--background, #ffffff))
      padding-box,
    conic-gradient(
      from var(--node--gradient-angle, 0deg),
      rgba(255, 109, 90, 1),
      rgba(255, 109, 90, 1) 20%,
      rgba(255, 109, 90, 0.2) 35%,
      rgba(255, 109, 90, 0.2) 65%,
      rgba(255, 109, 90, 1) 90%,
      rgba(255, 109, 90, 1)
    ) border-box;
}

:global(.dark) .workflow-node--running,
:global(.dark) .workflow-node--waiting {
  background:
    linear-gradient(hsl(0, 0%, 17%), hsl(0, 0%, 17%)) padding-box,
    conic-gradient(
      from var(--node--gradient-angle, 0deg),
      rgba(255, 109, 90, 1),
      rgba(255, 109, 90, 1) 20%,
      rgba(255, 109, 90, 0.2) 35%,
      rgba(255, 109, 90, 0.2) 65%,
      rgba(255, 109, 90, 1) 90%,
      rgba(255, 109, 90, 1)
    ) border-box;
}

@property --node--gradient-angle {
  syntax: '<angle>';
  initial-value: 0deg;
  inherits: false;
}

@keyframes border-rotate {
  from {
    --node--gradient-angle: 0deg;
  }
  to {
    --node--gradient-angle: 360deg;
  }
}

.workflow-node--running {
  animation: border-rotate 1.5s linear infinite;
}

.workflow-node--waiting {
  animation: border-rotate 4.5s linear infinite;
}

/*
 * Sticky note. The card fills the Vue Flow node wrapper — the wrapper
 * is sized from `parameters.width` / `parameters.height` (n8n keeps the
 * geometry in the node parameters) — so dragging the resize handle
 * updates the document and the card follows.
 *
 * The note body inherits `--sticky--color--text`, which the theme
 * layer flips together with the palette: Graphite text on the pale
 * light palette, Neutral text on the deep dark palette. The note body
 * therefore always clears 4.5:1 against its OWN fill, in both themes.
 */
.sticky-note {
  width: 100%;
  height: 100%;
  min-width: 120px;
  min-height: 80px;
  border-radius: 8px;
  border: 1px solid var(--border-color, oklch(0% 0 0 / 0.12));
  padding: 12px;
  overflow: hidden;
  position: relative;
  cursor: default;
  color: var(--sticky--color--text, var(--text-color, oklch(20.46% 0 89.88)));
}

.sticky-note.selected {
  box-shadow:
    0 0 0 calc(6px * var(--canvas-zoom-compensation-factor, 1)) hsla(220, 47%, 30%, 0.1);
}

.sticky-note__content {
  width: 100%;
  height: 100%;
  overflow: hidden;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  line-height: 1.45;
  color: inherit;
  cursor: text;
}

.sticky-note__editor {
  width: 100%;
  height: 100%;
  resize: none;
  border: none;
  outline: none;
  background: transparent;
  font-size: 13px;
  line-height: 1.45;
  color: inherit;
  font-family: inherit;
}

.sticky-note__colors {
  position: absolute;
  top: -34px;
  left: 0;
  display: flex;
  gap: 4px;
  padding: 4px;
  border-radius: 8px;
  background: var(--background--surface, #ffffff);
  border: 1px solid var(--border-color, oklch(0% 0 0 / 0.1));
  box-shadow: var(--shadow--sm, 0 4px 6px -1px oklch(0% 0 0 / 0.1));
  z-index: 10;
}

.sticky-note__swatch {
  width: 16px;
  height: 16px;
  border-radius: 9999px;
  border: 1px solid;
  cursor: pointer;
  padding: 0;
}

.sticky-note__swatch.selected {
  box-shadow: 0 0 0 2px var(--background--brand, oklch(78.07% 0.136 5.02));
}

/* Delete button — top-right corner, hidden by default, revealed on hover. */
.node-delete-btn {
  position: absolute;
  top: -8px;
  right: -8px;
  width: 20px;
  height: 20px;
  border-radius: 9999px;
  background: var(--color--danger, oklch(63.78% 0.2373 25.44));
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  opacity: 0;
  transform: scale(0.85);
  transition: opacity 120ms ease, transform 120ms ease;
  z-index: 5;
  box-shadow: var(--shadow--xs, 0 1px 2px -1px oklch(0% 0 0 / 0.1));
  cursor: pointer;
  border: 2px solid var(--background--surface, #ffffff);
}

.workflow-node:hover .node-delete-btn,
.workflow-node.selected .node-delete-btn,
.sticky-note:hover .node-delete-btn,
.sticky-note.selected .node-delete-btn {
  opacity: 1;
  transform: scale(1);
}

.node-delete-btn:hover {
  background: oklch(55.55% 0.2373 25.44);
  transform: scale(1.1);
}

/*
 * Connection handles.
 *
 * The <Handle> element is a 40px box centred on the node edge, so the
 * drop target stays generous, while the visible indicator is a 16px
 * dot — n8n's size. The dot is painted through `.handle-dot` so the
 * hover / valid / invalid states can recolour the indicator without
 * resizing the hit area.
 */
.connection-handle {
  width: 40px !important;
  height: 40px !important;
  background: transparent !important;
  border: none !important;
  border-radius: 0 !important;
  z-index: 2;
  position: absolute;
  transition: none;
}

.handle-dot {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 16px;
  height: 16px;
  border-radius: 9999px;
  background: var(--handle--color, oklch(68.3% 0 89.88));
  border: 2px solid var(--node--color--background, #ffffff);
  transition:
    background-color 200ms cubic-bezier(0.215, 0.61, 0.355, 1),
    transform 200ms cubic-bezier(0.215, 0.61, 0.355, 1);
}

:global(.dark) .handle-dot {
  border-color: var(--node--color--background, hsl(0, 0%, 17%));
  background: var(--handle--color, oklch(55.55% 0 89.88));
}

.connection-handle--input {
  /* Offset = half of the 40px handle so the dot sits on the node edge. */
  left: -20px !important;
}

.connection-handle--output {
  right: -20px !important;
}

.connection-handle:hover .handle-dot {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
  transform: translate(-50%, -50%) scale(1.15);
}

:global(.vue-flow__handle-connecting) .handle-dot,
:global(.vue-flow__handle-hot) .handle-dot {
  background: var(--background--brand, oklch(78.07% 0.136 5.02));
}

:global(.vue-flow__handle-valid) .handle-dot {
  background: var(--color--success, oklch(63.78% 0.155 155.83));
}

:global(.vue-flow__handle-invalid) .handle-dot {
  background: var(--color--danger, oklch(63.78% 0.2373 25.44));
}

@media (prefers-reduced-motion: reduce) {
  .workflow-node,
  .handle-dot,
  .node-delete-btn {
    transition-duration: 1ms;
  }

  .workflow-node--running,
  .workflow-node--waiting {
    animation: none;
  }
}
</style>
