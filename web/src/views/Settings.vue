<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import {
  SunIcon,
  MoonIcon,
  ComputerDesktopIcon,
  UserCircleIcon,
  ShieldCheckIcon,
  ChartBarIcon,
  CpuChipIcon,
} from '@heroicons/vue/24/outline'

import { useThemeStore, useAuthStore } from '@/stores'
import type { Theme } from '@/stores/theme'
import {
  getOtelConfig,
  updateOtelConfig,
  clearOtelConfig,
  sendOtelTestSpan,
} from '@/api/otel'
import type {
  OtelConfig,
  OtelConfigView,
  OtelOverride,
  OtelProtocol,
} from '@/types/otel'
import {
  getAIConfig,
  updateAIConfig,
  clearAIConfig,
  sendAITestChat,
} from '@/api/ai'
import type {
  AIConfig,
  AIConfigView,
  AIOverride,
  AIProviderId,
  AIProviderInfo,
} from '@/types/ai'

const themeStore = useThemeStore()
const authStore = useAuthStore()

const themes: { value: Theme; label: string; icon: typeof SunIcon }[] = [
  { value: 'light', label: 'Light', icon: SunIcon },
  { value: 'dark', label: 'Dark', icon: MoonIcon },
  { value: 'system', label: 'System', icon: ComputerDesktopIcon },
]

const profileForm = ref({
  firstName: '',
  lastName: '',
  email: '',
})

const passwordForm = ref({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})

// ---------------------------------------------------------------------------
// OpenTelemetry settings — env-as-default with a per-field DB override.
// ---------------------------------------------------------------------------

const otelLoading = ref(false)
const otelSaving = ref(false)
const otelTesting = ref(false)
const otelError = ref<string | null>(null)
const otelInfo = ref<string | null>(null)

// Tri-state fields the user can override. A field with `useEnv === true`
// falls back to whatever the env / dotenv said. The UI serializes only
// divergent fields so a partial override is preserved on save.
interface TriBool { value: boolean; useEnv: boolean }
interface TriStr  { value: string;  useEnv: boolean }
interface TriNum  { value: number;  useEnv: boolean }
interface TriProtocol { value: OtelProtocol; useEnv: boolean }

const otelEnabled         = ref<TriBool>({ value: false, useEnv: true })
const otelEndpoint        = ref<TriStr>({ value: '', useEnv: true })
const otelProtocol        = ref<TriProtocol>({ value: 'grpc', useEnv: true })
const otelHeaders         = ref<TriStr>({ value: '', useEnv: true })
const otelHeadersFile     = ref<TriStr>({ value: '', useEnv: true })
const otelSampleRate      = ref<TriNum>({ value: 1.0, useEnv: true })
const otelServiceName     = ref<TriStr>({ value: 'm9m', useEnv: true })
const otelServiceVersion  = ref<TriStr>({ value: '', useEnv: true })
const otelProductionOnly  = ref<TriBool>({ value: false, useEnv: true })
const otelIncludeNodeSpans = ref<TriBool>({ value: true, useEnv: true })
const otelInjectOutbound   = ref<TriBool>({ value: true, useEnv: true })
const otelAgentsEnabled    = ref<TriBool>({ value: false, useEnv: true })
const otelRecordInputs     = ref<TriBool>({ value: false, useEnv: true })
const otelRecordOutputs    = ref<TriBool>({ value: false, useEnv: true })

// The values the server actually applied. Useful for a "current effective
// config" summary the user can read at a glance.
const otelEffective = ref<OtelConfig | null>(null)
const otelEnvSource = ref<OtelConfig | null>(null)

const protocols: { value: OtelProtocol; label: string }[] = [
  { value: 'grpc', label: 'OTLP gRPC (port 4317)' },
  { value: 'http/protobuf', label: 'OTLP HTTP / protobuf (port 4318)' },
]

const overridesActive = computed(() => {
  const triFields = [
    otelEnabled, otelEndpoint, otelProtocol, otelHeaders, otelHeadersFile,
    otelSampleRate, otelServiceName, otelServiceVersion,
    otelProductionOnly, otelIncludeNodeSpans, otelInjectOutbound,
    otelAgentsEnabled, otelRecordInputs, otelRecordOutputs,
  ]
  return triFields.some((f) => !f.value.useEnv)
})

const loadOtel = async () => {
  otelLoading.value = true
  otelError.value = null
  try {
    const view: OtelConfigView = await getOtelConfig()
    applyViewToForm(view)
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    otelError.value = 'Failed to load OTEL config: ' + message
  } finally {
    otelLoading.value = false
  }
}

const applyViewToForm = (view: OtelConfigView) => {
  const { env, override, effective } = view
  otelEnvSource.value = env
  otelEffective.value = effective

  const ovBool = (b: boolean | null | undefined, fallback: boolean): TriBool => ({
    value: b ?? fallback,
    useEnv: b === null || b === undefined,
  })
  const ovStr = (s: string | null | undefined, fallback: string): TriStr => ({
    value: s ?? fallback,
    useEnv: s === null || s === undefined,
  })
  const ovNum = (n: number | null | undefined, fallback: number): TriNum => ({
    value: n ?? fallback,
    useEnv: n === null || n === undefined,
  })
  const ovProto = (s: string | null | undefined, fallback: OtelProtocol): TriProtocol => {
    const value: OtelProtocol = s === 'http/protobuf' ? 'http/protobuf' : (s === 'grpc' ? 'grpc' : fallback)
    return {
      value,
      useEnv: s === null || s === undefined,
    }
  }

  otelEnabled.value          = ovBool(override.enabled, env.enabled)
  otelEndpoint.value         = ovStr(override.endpoint, env.endpoint)
  otelProtocol.value         = ovProto(override.protocol, env.protocol)
  otelHeaders.value          = ovStr(override.headers, env.headers)
  otelHeadersFile.value      = ovStr(override.headersFile, env.headersFile)
  otelSampleRate.value       = ovNum(override.sampleRate, env.sampleRate)
  otelServiceName.value      = ovStr(override.serviceName, env.serviceName)
  otelServiceVersion.value   = ovStr(override.serviceVersion, env.serviceVersion)
  otelProductionOnly.value   = ovBool(override.productionOnly, env.productionOnly)
  otelIncludeNodeSpans.value = ovBool(override.includeNodeSpans, env.includeNodeSpans)
  otelInjectOutbound.value   = ovBool(override.injectOutbound, env.injectOutbound)
  otelAgentsEnabled.value    = ovBool(override.agentsEnabled, env.agentsEnabled)
  otelRecordInputs.value     = ovBool(override.agentsRecordInputs, env.agentsRecordInputs)
  otelRecordOutputs.value    = ovBool(override.agentsRecordOutputs, env.agentsRecordOutputs)
}

const buildOverride = (): OtelOverride => {
  // Only fields the user explicitly diverged from env are included.
  // This matches the Go side's tri-state semantics (nil = "follow env").
  const ov: OtelOverride = {}
  if (!otelEnabled.value.useEnv) ov.enabled = otelEnabled.value.value
  if (!otelEndpoint.value.useEnv) ov.endpoint = otelEndpoint.value.value
  if (!otelProtocol.value.useEnv) ov.protocol = otelProtocol.value.value
  if (!otelHeaders.value.useEnv) ov.headers = otelHeaders.value.value
  if (!otelHeadersFile.value.useEnv) ov.headersFile = otelHeadersFile.value.value
  if (!otelSampleRate.value.useEnv) ov.sampleRate = otelSampleRate.value.value
  if (!otelServiceName.value.useEnv) ov.serviceName = otelServiceName.value.value
  if (!otelServiceVersion.value.useEnv) ov.serviceVersion = otelServiceVersion.value.value
  if (!otelProductionOnly.value.useEnv) ov.productionOnly = otelProductionOnly.value.value
  if (!otelIncludeNodeSpans.value.useEnv) ov.includeNodeSpans = otelIncludeNodeSpans.value.value
  if (!otelInjectOutbound.value.useEnv) ov.injectOutbound = otelInjectOutbound.value.value
  if (!otelAgentsEnabled.value.useEnv) ov.agentsEnabled = otelAgentsEnabled.value.value
  if (!otelRecordInputs.value.useEnv) ov.agentsRecordInputs = otelRecordInputs.value.value
  if (!otelRecordOutputs.value.useEnv) ov.agentsRecordOutputs = otelRecordOutputs.value.value
  return ov
}

const saveOtel = async () => {
  otelSaving.value = true
  otelError.value = null
  otelInfo.value = null
  try {
    const override = buildOverride()
    const view = await updateOtelConfig(override)
    applyViewToForm(view)
    otelInfo.value = 'OpenTelemetry settings saved. Tracer provider reloaded.'
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    otelError.value = 'Failed to save OTEL config: ' + message
  } finally {
    otelSaving.value = false
  }
}

const resetOtelToEnv = async () => {
  otelSaving.value = true
  otelError.value = null
  otelInfo.value = null
  try {
    const view = await clearOtelConfig()
    applyViewToForm(view)
    otelInfo.value = 'Restored env defaults. Tracer provider reloaded.'
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    otelError.value = 'Failed to reset OTEL config: ' + message
  } finally {
    otelSaving.value = false
  }
}

const testOtel = async () => {
  otelTesting.value = true
  otelError.value = null
  otelInfo.value = null
  try {
    await sendOtelTestSpan()
    otelInfo.value =
      'Smoke span dispatched. Check your collector UI for service "' +
      (otelEffective.value?.serviceName || 'm9m') +
      '" within 5-15 seconds.'
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    otelError.value = 'Test span failed: ' + message
  } finally {
    otelTesting.value = false
  }
}

// ---------------------------------------------------------------------------
// AI assistant settings — env-as-default with a per-field DB override.
// ---------------------------------------------------------------------------

const aiLoading = ref(false)
const aiSaving = ref(false)
const aiTesting = ref(false)
const aiError = ref<string | null>(null)
const aiInfo = ref<string | null>(null)
const aiTestReply = ref<string | null>(null)
const aiTestPrompt = ref('')

// Tri-state fields for the AI override UI. Same shape as the OTEL
// block: every field has a "use env" toggle that gates the value
// input. When `useEnv` is true the corresponding key in the override
// payload is omitted (the backend treats absent keys as "no override").
interface AITriBool { value: boolean; useEnv: boolean }
interface AITriStr  { value: string;  useEnv: boolean }
interface AITriNum  { value: number;  useEnv: boolean }
interface AITriProvider { value: AIProviderId; useEnv: boolean }

const aiEnabled     = ref<AITriBool>({ value: false, useEnv: true })
const aiProvider    = ref<AITriProvider>({ value: 'openai', useEnv: true })
const aiApiKey      = ref<AITriStr>({ value: '', useEnv: true })
const aiBaseUrl     = ref<AITriStr>({ value: '', useEnv: true })
const aiModel       = ref<AITriStr>({ value: '', useEnv: true })
const aiMaxTokens   = ref<AITriNum>({ value: 4096, useEnv: true })
const aiTemperature = ref<AITriNum>({ value: 0.7, useEnv: true })
const aiTimeoutMs   = ref<AITriNum>({ value: 60000, useEnv: true })

const aiEffective = ref<AIConfig | null>(null)
const aiEnvSource = ref<AIConfig | null>(null)
const aiProviders = ref<AIProviderInfo[]>([])

const aiSelectedProviderInfo = computed<AIProviderInfo | null>(() => {
  const id = aiProvider.value.value
  return aiProviders.value.find((p) => p.id === id) ?? null
})

const aiModelOptions = computed<string[]>(() => {
  // Prefer the catalog's per-provider model list; fall back to the
  // currently-typed value so a custom model id is still accepted.
  const list = aiSelectedProviderInfo.value?.models ?? []
  if (!aiModel.value.useEnv && aiModel.value.value && !list.includes(aiModel.value.value)) {
    return [aiModel.value.value, ...list]
  }
  return list
})

const aiOverridesActive = computed(() => {
  const tri = [aiEnabled, aiProvider, aiApiKey, aiBaseUrl, aiModel, aiMaxTokens, aiTemperature, aiTimeoutMs]
  return tri.some((f) => !f.value.useEnv)
})

// When the operator picks a different provider, snap the model field
// to the new provider's default — unless the operator had manually
// overridden the model (in which case we leave their text alone).
watch(
  () => aiProvider.value.value,
  (newId) => {
    if (aiModel.value.useEnv) {
      const info = aiProviders.value.find((p) => p.id === newId)
      if (info) {
        aiModel.value.value = info.defaultModel
      }
    }
    if (aiBaseUrl.value.useEnv) {
      const info = aiProviders.value.find((p) => p.id === newId)
      if (info) {
        aiBaseUrl.value.value = info.defaultBaseUrl
      }
    }
  }
)

const loadAI = async () => {
  aiLoading.value = true
  aiError.value = null
  aiInfo.value = null
  aiTestReply.value = null
  try {
    const view: AIConfigView = await getAIConfig()
    applyAIViewToForm(view)
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    aiError.value = 'Failed to load AI config: ' + message
  } finally {
    aiLoading.value = false
  }
}

const applyAIViewToForm = (view: AIConfigView) => {
  const { env, override, effective, providers } = view
  aiEnvSource.value = env
  aiEffective.value = effective
  aiProviders.value = providers

  const ovBool = (b: boolean | null | undefined, fallback: boolean): AITriBool => ({
    value: b ?? fallback,
    useEnv: b === null || b === undefined,
  })
  const ovStr = (s: string | null | undefined, fallback: string): AITriStr => ({
    value: s ?? fallback,
    useEnv: s === null || s === undefined,
  })
  const ovNum = (n: number | null | undefined, fallback: number): AITriNum => ({
    value: n ?? fallback,
    useEnv: n === null || n === undefined,
  })
  const ovProvider = (s: string | null | undefined, fallback: AIProviderId): AITriProvider => {
    const known: AIProviderId[] = ['openai', 'anthropic', 'minimax', 'ollama']
    const value: AIProviderId = (known as string[]).includes(s ?? '')
      ? (s as AIProviderId)
      : fallback
    return { value, useEnv: s === null || s === undefined }
  }

  aiEnabled.value     = ovBool(override.enabled, env.enabled)
  aiProvider.value    = ovProvider(override.provider, env.provider)
  aiApiKey.value      = ovStr(override.apiKey, env.apiKey)
  aiBaseUrl.value     = ovStr(override.baseUrl, env.baseUrl)
  aiModel.value       = ovStr(override.model, env.model)
  aiMaxTokens.value   = ovNum(override.maxTokens, env.maxTokens)
  aiTemperature.value = ovNum(override.temperature, env.temperature)
  aiTimeoutMs.value   = ovNum(override.timeoutMs, env.timeoutMs)
}

const buildAIOverride = (): AIOverride => {
  const ov: AIOverride = { source: 'ui' }
  if (!aiEnabled.value.useEnv) ov.enabled = aiEnabled.value.value
  if (!aiProvider.value.useEnv) ov.provider = aiProvider.value.value
  if (!aiApiKey.value.useEnv) ov.apiKey = aiApiKey.value.value
  if (!aiBaseUrl.value.useEnv) ov.baseUrl = aiBaseUrl.value.value
  if (!aiModel.value.useEnv) ov.model = aiModel.value.value
  if (!aiMaxTokens.value.useEnv) ov.maxTokens = aiMaxTokens.value.value
  if (!aiTemperature.value.useEnv) ov.temperature = aiTemperature.value.value
  if (!aiTimeoutMs.value.useEnv) ov.timeoutMs = aiTimeoutMs.value.value
  return ov
}

const saveAI = async () => {
  aiSaving.value = true
  aiError.value = null
  aiInfo.value = null
  try {
    const override = buildAIOverride()
    const view = await updateAIConfig(override)
    applyAIViewToForm(view)
    aiInfo.value = 'AI settings saved. Runtime reloaded live — no restart required.'
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    aiError.value = 'Failed to save AI config: ' + message
  } finally {
    aiSaving.value = false
  }
}

const resetAIToEnv = async () => {
  aiSaving.value = true
  aiError.value = null
  aiInfo.value = null
  try {
    const view = await clearAIConfig()
    applyAIViewToForm(view)
    aiInfo.value = 'Restored env defaults. Runtime reloaded live.'
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    aiError.value = 'Failed to reset AI config: ' + message
  } finally {
    aiSaving.value = false
  }
}

const testAI = async () => {
  aiTesting.value = true
  aiError.value = null
  aiInfo.value = null
  aiTestReply.value = null
  try {
    const prompt = aiTestPrompt.value.trim() ||
      'Reply with exactly: m9m AI assistant ready.'
    const reply = await sendAITestChat(prompt)
    aiTestReply.value = `${reply.message}`
    aiInfo.value = `Test chat succeeded in ${reply.elapsed}.`
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : 'Unknown error'
    aiError.value = 'Test chat failed: ' + message
  } finally {
    aiTesting.value = false
  }
}

onMounted(() => {
  if (authStore.user) {
    profileForm.value = {
      firstName: authStore.user.firstName || '',
      lastName: authStore.user.lastName || '',
      email: authStore.user.email,
    }
  }
  loadOtel()
  loadAI()
})

const saveProfile = async () => {
  try {
    await authStore.updateProfile({
      firstName: profileForm.value.firstName,
      lastName: profileForm.value.lastName,
    })
    alert('Profile updated successfully')
  } catch (e) {
    console.error('Failed to update profile:', e)
    alert('Failed to update profile. Please try again.')
  }
}

const changePassword = async () => {
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    alert('New passwords do not match')
    return
  }

  try {
    await authStore.changePassword(
      passwordForm.value.currentPassword,
      passwordForm.value.newPassword
    )
    passwordForm.value = { currentPassword: '', newPassword: '', confirmPassword: '' }
    alert('Password changed successfully')
  } catch (e) {
    console.error('Failed to change password:', e)
    alert('Failed to change password. Please check your current password.')
  }
}
</script>

<template>
  <div class="p-6 max-w-4xl mx-auto">
    <h1 class="text-2xl font-bold text-slate-900 dark:text-white mb-6">Settings</h1>

    <div class="space-y-6">
      <!-- Appearance -->
      <div class="card p-6">
        <div class="flex items-center gap-3 mb-4">
          <SunIcon class="w-5 h-5 text-slate-500" />
          <h2 class="text-lg font-semibold text-slate-900 dark:text-white">Appearance</h2>
        </div>

        <div class="space-y-4">
          <div>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
              Theme
            </label>
            <div class="flex gap-3">
              <button
                v-for="theme in themes"
                :key="theme.value"
                @click="themeStore.setTheme(theme.value)"
                :class="[
                  'flex items-center gap-2 px-4 py-2 rounded-lg border-2 transition-colors',
                  themeStore.theme === theme.value
                    ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-400'
                    : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-400 hover:border-slate-300 dark:hover:border-slate-600'
                ]"
              >
                <component :is="theme.icon" class="w-5 h-5" />
                <span>{{ theme.label }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Telemetry (OpenTelemetry) -->
      <div class="card p-6">
        <div class="flex items-center gap-3 mb-4">
          <ChartBarIcon class="w-5 h-5 text-slate-500" />
          <h2 class="text-lg font-semibold text-slate-900 dark:text-white">Telemetry</h2>
          <span class="ml-auto text-xs uppercase tracking-wide px-2 py-0.5 rounded-full"
                :class="otelEffective?.enabled
                  ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-200'
                  : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'">
            {{ otelEffective?.enabled ? 'enabled' : 'disabled' }}
          </span>
        </div>

        <p class="text-sm text-slate-600 dark:text-slate-400 mb-4">
          OpenTelemetry traces are exported via OTLP to any OTel-compatible
          collector (Jaeger, SigNoz, Tempo, Honeycomb, Datadog, etc.).
          Settings below <strong>override</strong> the environment defaults —
          leave a field's checkbox unchecked to keep using the value from
          your shell / <code>.env</code>.
        </p>

        <!-- Master switch -->
        <div class="grid grid-cols-12 gap-4 items-start mb-4">
          <div class="col-span-12">
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="otelEnabled.useEnv"
                @change="otelEnabled.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Use environment default for <strong>enabled</strong>
              </span>
            </label>
            <label class="flex items-center gap-3 mt-1 pl-6">
              <input
                type="checkbox"
                v-model="otelEnabled.value"
                :disabled="otelEnabled.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Export traces to the configured collector
              </span>
            </label>
          </div>
        </div>

        <!-- Endpoint + protocol -->
        <div class="grid grid-cols-12 gap-4 items-start border-t border-slate-100 dark:border-slate-800 pt-4 mb-4">
          <div class="col-span-12 md:col-span-8">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              OTLP endpoint
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="otelEndpoint.useEnv"
                @change="otelEndpoint.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_OTEL_ENDPOINT)</span>
            </label>
            <input
              v-model="otelEndpoint.value"
              :disabled="otelEndpoint.useEnv"
              type="text"
              :placeholder="otelEnvSource?.endpoint || 'host:4317'"
              class="input font-mono text-sm"
            />
          </div>

          <div class="col-span-12 md:col-span-4">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Protocol
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="otelProtocol.useEnv"
                @change="otelProtocol.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <select
              :value="otelProtocol.value"
              :disabled="otelProtocol.useEnv"
              @change="otelProtocol.value = ($event.target as HTMLSelectElement).value as OtelProtocol"
              class="input"
            >
              <option v-for="p in protocols" :key="p.value" :value="p.value">{{ p.label }}</option>
            </select>
          </div>
        </div>

        <!-- Headers -->
        <div class="grid grid-cols-12 gap-4 items-start mb-4">
          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Headers
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="otelHeaders.useEnv"
                @change="otelHeaders.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_OTEL_HEADERS)</span>
            </label>
            <input
              v-model="otelHeaders.value"
              :disabled="otelHeaders.useEnv"
              type="text"
              placeholder="Authorization=Bearer xyz"
              class="input font-mono text-sm"
            />
          </div>
          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Headers file
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="otelHeadersFile.useEnv"
                @change="otelHeadersFile.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <input
              v-model="otelHeadersFile.value"
              :disabled="otelHeadersFile.useEnv"
              type="text"
              placeholder="/etc/m9m/otel-headers.json"
              class="input font-mono text-sm"
            />
          </div>
        </div>

        <!-- Service identity — the value your backend labels traces with. -->
        <div class="grid grid-cols-12 gap-4 items-start border-t border-slate-100 dark:border-slate-800 pt-4 mb-4">
          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Service name
              <span class="text-xs text-slate-500 ml-2">
                (becomes <code>service.name</code> in every span)
              </span>
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="otelServiceName.useEnv"
                @change="otelServiceName.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_OTEL_SERVICE_NAME)</span>
            </label>
            <input
              v-model="otelServiceName.value"
              :disabled="otelServiceName.useEnv"
              type="text"
              placeholder="m9m"
              class="input font-mono text-sm"
            />
          </div>

          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Service version
              <span class="text-xs text-slate-500 ml-2">
                (becomes <code>service.version</code>)
              </span>
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="otelServiceVersion.useEnv"
                @change="otelServiceVersion.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_OTEL_SERVICE_VERSION)</span>
            </label>
            <input
              v-model="otelServiceVersion.value"
              :disabled="otelServiceVersion.useEnv"
              type="text"
              placeholder="1.0.0"
              class="input font-mono text-sm"
            />
          </div>
        </div>

        <!-- Sampling + toggles -->
        <div class="grid grid-cols-12 gap-4 items-start border-t border-slate-100 dark:border-slate-800 pt-4 mb-4">
          <div class="col-span-12 md:col-span-4">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Sample rate
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="otelSampleRate.useEnv"
                @change="otelSampleRate.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <input
              v-model.number="otelSampleRate.value"
              :disabled="otelSampleRate.useEnv"
              type="number"
              step="0.05"
              min="0"
              max="1"
              class="input font-mono text-sm"
            />
          </div>

          <div class="col-span-12 md:col-span-8 grid grid-cols-1 gap-2">
            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                :checked="otelProductionOnly.useEnv"
                @change="otelProductionOnly.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <label class="flex items-center gap-3 -mt-5">
              <input
                type="checkbox"
                v-model="otelProductionOnly.value"
                :disabled="otelProductionOnly.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Drop dev / test execution modes at sampling
              </span>
            </label>

            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                :checked="otelIncludeNodeSpans.useEnv"
                @change="otelIncludeNodeSpans.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <label class="flex items-center gap-3 -mt-5">
              <input
                type="checkbox"
                v-model="otelIncludeNodeSpans.value"
                :disabled="otelIncludeNodeSpans.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Emit per-node <code>node.execute</code> spans
              </span>
            </label>

            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                :checked="otelInjectOutbound.useEnv"
                @change="otelInjectOutbound.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <label class="flex items-center gap-3 -mt-5">
              <input
                type="checkbox"
                v-model="otelInjectOutbound.value"
                :disabled="otelInjectOutbound.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Inject traceparent into outbound HTTP requests
              </span>
            </label>
          </div>
        </div>

        <!-- AI agent tracing -->
        <details class="border-t border-slate-100 dark:border-slate-800 pt-4">
          <summary class="cursor-pointer text-sm font-medium text-slate-700 dark:text-slate-300">
            AI agent tracing (OpenAI / Anthropic / etc.)
          </summary>
          <div class="space-y-2 mt-3">
            <label class="flex items-center gap-2">
              <input
                type="checkbox"
                :checked="otelAgentsEnabled.useEnv"
                @change="otelAgentsEnabled.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AGENTS_TRACING_ENABLED)</span>
            </label>
            <label class="flex items-center gap-3">
              <input
                type="checkbox"
                v-model="otelAgentsEnabled.value"
                :disabled="otelAgentsEnabled.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Emit <code>&lt;agent&gt;.generate</code> spans for AI calls
              </span>
            </label>

            <label class="flex items-center gap-2 mt-2">
              <input
                type="checkbox"
                :checked="otelRecordInputs.useEnv"
                @change="otelRecordInputs.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <label class="flex items-center gap-3">
              <input
                type="checkbox"
                v-model="otelRecordInputs.value"
                :disabled="otelRecordInputs.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Record prompts (<code>gen_ai.prompt</code>)
              </span>
            </label>

            <label class="flex items-center gap-2 mt-2">
              <input
                type="checkbox"
                :checked="otelRecordOutputs.useEnv"
                @change="otelRecordOutputs.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default</span>
            </label>
            <label class="flex items-center gap-3">
              <input
                type="checkbox"
                v-model="otelRecordOutputs.value"
                :disabled="otelRecordOutputs.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Record responses (<code>gen_ai.response</code>)
              </span>
            </label>
            <p class="text-xs text-slate-500 mt-1">
              Privacy gate — leave OFF unless your collector is GDPR-compliant
              and you have an explicit business reason to record LLM traffic.
            </p>
          </div>
        </details>

        <!-- Status + actions -->
        <div v-if="otelError" class="mt-4 p-3 rounded-md bg-red-50 dark:bg-red-900/30 text-red-800 dark:text-red-200 text-sm">
          {{ otelError }}
        </div>
        <div v-if="otelInfo" class="mt-4 p-3 rounded-md bg-emerald-50 dark:bg-emerald-900/30 text-emerald-800 dark:text-emerald-200 text-sm">
          {{ otelInfo }}
        </div>

        <div class="flex flex-wrap gap-3 mt-4">
          <button
            type="button"
            class="btn-primary"
            :disabled="otelSaving || otelLoading"
            @click="saveOtel"
          >
            {{ otelSaving ? 'Saving...' : 'Save settings' }}
          </button>
          <button
            type="button"
            class="btn-secondary"
            :disabled="otelSaving || !overridesActive"
            @click="resetOtelToEnv"
          >
            Reset to env defaults
          </button>
          <button
            type="button"
            class="btn-secondary"
            :disabled="otelTesting || !otelEffective?.enabled"
            @click="testOtel"
          >
            {{ otelTesting ? 'Sending...' : 'Send test span' }}
          </button>
          <span v-if="otelEffective" class="ml-auto text-xs text-slate-500 dark:text-slate-400 self-center">
            Effective: {{ otelEffective.protocol }} → {{ otelEffective.endpoint }} · sample {{ otelEffective.sampleRate }}
          </span>
        </div>
      </div>

      <!-- AI assistant -->
      <div class="card p-6">
        <div class="flex items-center gap-3 mb-4">
          <CpuChipIcon class="w-5 h-5 text-slate-500" />
          <h2 class="text-lg font-semibold text-slate-900 dark:text-white">AI</h2>
          <span class="ml-auto text-xs uppercase tracking-wide px-2 py-0.5 rounded-full"
                :class="aiEffective?.enabled
                  ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-200'
                  : 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'">
            {{ aiEffective?.enabled ? 'enabled' : 'disabled' }}
          </span>
        </div>

        <p class="text-sm text-slate-600 dark:text-slate-400 mb-4">
          Configure the AI assistant that powers workflow generation,
          chat, and node suggestions. Settings below <strong>override</strong>
          the environment defaults — leave a field's checkbox unchecked to
          keep the value from your shell / <code>.env</code>.
          Supports OpenAI, Anthropic (Claude), MiniMax (Anthropic-compatible),
          and Ollama.
        </p>

        <!-- Master switch -->
        <div class="grid grid-cols-12 gap-4 items-start mb-4">
          <div class="col-span-12">
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="aiEnabled.useEnv"
                @change="aiEnabled.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Use environment default for <strong>enabled</strong>
                <span class="text-xs text-slate-500 ml-1">(M9M_AI_ENABLED)</span>
              </span>
            </label>
            <label class="flex items-center gap-3 mt-1 pl-6">
              <input
                type="checkbox"
                v-model="aiEnabled.value"
                :disabled="aiEnabled.useEnv"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-sm text-slate-700 dark:text-slate-300">
                Enable AI assistant
              </span>
            </label>
          </div>
        </div>

        <!-- Provider + Model -->
        <div class="grid grid-cols-12 gap-4 items-start border-t border-slate-100 dark:border-slate-800 pt-4 mb-4">
          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Provider
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="aiProvider.useEnv"
                @change="aiProvider.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AI_PROVIDER)</span>
            </label>
            <select
              :value="aiProvider.value"
              :disabled="aiProvider.useEnv"
              @change="aiProvider.value = ($event.target as HTMLSelectElement).value as AIProviderId"
              class="input"
            >
              <option
                v-for="p in aiProviders"
                :key="p.id"
                :value="p.id"
              >
                {{ p.label }}
              </option>
            </select>
            <p v-if="aiSelectedProviderInfo" class="mt-1 text-xs text-slate-500">
              {{ aiSelectedProviderInfo.description }}
            </p>
          </div>

          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Model
              <span class="text-xs text-slate-500 ml-2">
                (provider-aware default below)
              </span>
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="aiModel.useEnv"
                @change="aiModel.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AI_MODEL)</span>
            </label>
            <input
              v-model="aiModel.value"
              :disabled="aiModel.useEnv"
              type="text"
              :list="aiProvider.useEnv ? undefined : 'ai-model-options'"
              :placeholder="aiSelectedProviderInfo?.defaultModel || 'model id'"
              class="input font-mono text-sm"
            />
            <datalist id="ai-model-options">
              <option
                v-for="m in aiModelOptions"
                :key="m"
                :value="m"
              />
            </datalist>
            <p v-if="aiModelOptions.length" class="mt-1 text-xs text-slate-500">
              {{ aiModelOptions.length }} known models for {{ aiSelectedProviderInfo?.label || aiProvider.value }}
            </p>
          </div>
        </div>

        <!-- API key + base URL -->
        <div class="grid grid-cols-12 gap-4 items-start border-t border-slate-100 dark:border-slate-800 pt-4 mb-4">
          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              API key
              <span v-if="aiSelectedProviderInfo && !aiSelectedProviderInfo.requiresKey" class="text-xs text-emerald-600 ml-2">
                (not required for {{ aiSelectedProviderInfo.label }})
              </span>
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="aiApiKey.useEnv"
                @change="aiApiKey.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AI_API_KEY)</span>
            </label>
            <input
              v-model="aiApiKey.value"
              :disabled="aiApiKey.useEnv"
              type="password"
              autocomplete="off"
              placeholder="sk-…"
              class="input font-mono text-sm"
            />
          </div>

          <div class="col-span-12 md:col-span-6">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Base URL
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="aiBaseUrl.useEnv"
                @change="aiBaseUrl.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AI_BASE_URL)</span>
            </label>
            <input
              v-model="aiBaseUrl.value"
              :disabled="aiBaseUrl.useEnv"
              type="text"
              :placeholder="aiSelectedProviderInfo?.defaultBaseUrl || 'https://…'"
              class="input font-mono text-sm"
            />
            <p v-if="aiBaseUrl.useEnv && aiEnvSource?.baseUrl" class="mt-1 text-xs text-slate-500">
              Currently using: <code>{{ aiEnvSource.baseUrl }}</code>
            </p>
          </div>
        </div>

        <!-- Numeric knobs -->
        <div class="grid grid-cols-12 gap-4 items-start border-t border-slate-100 dark:border-slate-800 pt-4 mb-4">
          <div class="col-span-12 md:col-span-4">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Max tokens
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="aiMaxTokens.useEnv"
                @change="aiMaxTokens.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AI_MAX_TOKENS)</span>
            </label>
            <input
              v-model.number="aiMaxTokens.value"
              :disabled="aiMaxTokens.useEnv"
              type="number"
              min="1"
              max="200000"
              class="input font-mono text-sm"
            />
          </div>

          <div class="col-span-12 md:col-span-4">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Temperature
              <span class="text-xs text-slate-500 ml-2">(0.0–2.0)</span>
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="aiTemperature.useEnv"
                @change="aiTemperature.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AI_TEMPERATURE)</span>
            </label>
            <input
              v-model.number="aiTemperature.value"
              :disabled="aiTemperature.useEnv"
              type="number"
              step="0.05"
              min="0"
              max="2"
              class="input font-mono text-sm"
            />
          </div>

          <div class="col-span-12 md:col-span-4">
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Timeout (ms)
            </label>
            <label class="flex items-center gap-2 mb-1">
              <input
                type="checkbox"
                :checked="aiTimeoutMs.useEnv"
                @change="aiTimeoutMs.useEnv = ($event.target as HTMLInputElement).checked"
                class="rounded border-slate-300 text-primary-600 focus:ring-primary-500"
              />
              <span class="text-xs text-slate-500">Use env default (M9M_AI_TIMEOUT_MS)</span>
            </label>
            <input
              v-model.number="aiTimeoutMs.value"
              :disabled="aiTimeoutMs.useEnv"
              type="number"
              min="100"
              max="600000"
              step="100"
              class="input font-mono text-sm"
            />
          </div>
        </div>

        <!-- Test chat -->
        <details class="border-t border-slate-100 dark:border-slate-800 pt-4">
          <summary class="cursor-pointer text-sm font-medium text-slate-700 dark:text-slate-300">
            Send test chat (uses current effective config)
          </summary>
          <div class="space-y-3 mt-3">
            <input
              v-model="aiTestPrompt"
              type="text"
              placeholder="Reply with exactly: m9m AI assistant ready."
              class="input font-mono text-sm"
            />
            <div v-if="aiTestReply" class="p-3 rounded-md bg-slate-50 dark:bg-slate-900 text-sm">
              <span class="text-xs text-slate-500">Reply:</span>
              <p class="mt-1 whitespace-pre-wrap">{{ aiTestReply }}</p>
            </div>
          </div>
        </details>

        <!-- Status + actions -->
        <div v-if="aiError" class="mt-4 p-3 rounded-md bg-red-50 dark:bg-red-900/30 text-red-800 dark:text-red-200 text-sm">
          {{ aiError }}
        </div>
        <div v-if="aiInfo" class="mt-4 p-3 rounded-md bg-emerald-50 dark:bg-emerald-900/30 text-emerald-800 dark:text-emerald-200 text-sm">
          {{ aiInfo }}
        </div>

        <div class="flex flex-wrap gap-3 mt-4">
          <button
            type="button"
            class="btn-primary"
            :disabled="aiSaving || aiLoading"
            @click="saveAI"
          >
            {{ aiSaving ? 'Saving...' : 'Save settings' }}
          </button>
          <button
            type="button"
            class="btn-secondary"
            :disabled="aiSaving || !aiOverridesActive"
            @click="resetAIToEnv"
          >
            Reset to env defaults
          </button>
          <button
            type="button"
            class="btn-secondary"
            :disabled="aiTesting || !aiEffective?.enabled"
            @click="testAI"
          >
            {{ aiTesting ? 'Sending...' : 'Send test chat' }}
          </button>
          <span v-if="aiEffective" class="ml-auto text-xs text-slate-500 dark:text-slate-400 self-center">
            Effective: {{ aiEffective.provider }} → {{ aiEffective.model || '(default)' }}
            @ {{ aiEffective.baseUrl || '(provider default)' }}
          </span>
        </div>
      </div>

      <!-- Profile -->
      <div v-if="authStore.isAuthenticated" class="card p-6">
        <div class="flex items-center gap-3 mb-4">
          <UserCircleIcon class="w-5 h-5 text-slate-500" />
          <h2 class="text-lg font-semibold text-slate-900 dark:text-white">Profile</h2>
        </div>

        <form @submit.prevent="saveProfile" class="space-y-4">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                First Name
              </label>
              <input
                v-model="profileForm.firstName"
                type="text"
                class="input"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                Last Name
              </label>
              <input
                v-model="profileForm.lastName"
                type="text"
                class="input"
              />
            </div>
          </div>

          <div>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Email
            </label>
            <input
              v-model="profileForm.email"
              type="email"
              class="input"
              disabled
            />
          </div>

          <button type="submit" class="btn-primary">
            Save Profile
          </button>
        </form>
      </div>

      <!-- Security -->
      <div v-if="authStore.isAuthenticated" class="card p-6">
        <div class="flex items-center gap-3 mb-4">
          <ShieldCheckIcon class="w-5 h-5 text-slate-500" />
          <h2 class="text-lg font-semibold text-slate-900 dark:text-white">Security</h2>
        </div>

        <form @submit.prevent="changePassword" class="space-y-4">
          <div>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Current Password
            </label>
            <input
              v-model="passwordForm.currentPassword"
              type="password"
              class="input"
              required
            />
          </div>

          <div>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              New Password
            </label>
            <input
              v-model="passwordForm.newPassword"
              type="password"
              class="input"
              required
            />
          </div>

          <div>
            <label class="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
              Confirm New Password
            </label>
            <input
              v-model="passwordForm.confirmPassword"
              type="password"
              class="input"
              required
            />
          </div>

          <button type="submit" class="btn-primary">
            Change Password
          </button>
        </form>
      </div>

      <!-- About -->
      <div class="card p-6">
        <h2 class="text-lg font-semibold text-slate-900 dark:text-white mb-4">About</h2>
        <div class="space-y-2 text-sm text-slate-600 dark:text-slate-400">
          <p><span class="font-medium">m9m</span> v1.0.0</p>
          <p>Agent-Native Workflow Automation Platform</p>
          <p class="mt-4">
            <a
              href="https://github.com/neul-labs/m9m"
              target="_blank"
              class="text-primary-600 dark:text-primary-400 hover:underline"
            >
              GitHub Repository
            </a>
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
