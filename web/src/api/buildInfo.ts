import apiClient from './client'

// BuildInfo mirrors the JSON returned by GET /api/v1/version. The legacy
// n8n-compatible fields (n8nVersion / serverVersion / implementation /
// compatibility) are preserved for the parity suite + sync bridge; the
// three "stamp" fields — version, commit, buildDate — are populated by
// the Dockerfile at build time via `-ldflags -X main.Commit=...` and are
// what the sidebar footer renders so users can verify the running
// container is on the latest push.
//
// All three stamp fields fall back to "unknown" when SetBuildInfo was
// never called on the server (e.g. a unit test backend). Callers should
// treat that as "dev / can't tell" and either hide the chip or show it
// dim.
export interface BuildInfo {
  n8nVersion: string
  serverVersion: string
  implementation: string
  compatibility: Record<string, boolean>
  version: string
  commit: string
  buildDate: string
}

// GET /api/v1/version — returns the running binary's source identity.
// Used by the sidebar footer to show "vX.Y.Z @ <shortsha> • <date>".
export async function getBuildInfo(): Promise<BuildInfo> {
  const response = await apiClient.get<BuildInfo>('/version')
  return response.data
}
