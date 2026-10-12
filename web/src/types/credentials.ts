// Credential types shared between the form (credentialSchema.ts)
// and the editor / views. The `CredentialSchema` /
// `CredentialSchemaField` shapes match the Go side
// (`internal/credentials/oauth2.go`).

export interface CredentialSchemaField {
  name: string
  display: string
  type: 'string' | 'password' | 'number' | 'options'
  required?: boolean
  placeholder?: string
  default?: string
  options?: string[]
}

export interface CredentialSchema {
  type: string
  display: string
  properties: CredentialSchemaField[]
}

// OAuth2Schema mirrors `internal/credentials/oauth2.go::OAuth2Schema`.
// The field names MUST stay in sync so the wire format
// round-trips.
export const OAuth2Schema: CredentialSchema = {
  type: 'oauth2Api',
  display: 'OAuth2 API',
  properties: [
    { name: 'clientId', display: 'Client ID', type: 'string', required: true },
    { name: 'clientSecret', display: 'Client Secret', type: 'password', required: true },
    { name: 'authUrl', display: 'Authorization URL', type: 'string', required: true, placeholder: 'https://example.com/oauth/authorize' },
    { name: 'accessTokenUrl', display: 'Access Token URL', type: 'string', required: true, placeholder: 'https://example.com/oauth/token' },
    { name: 'scope', display: 'Scope', type: 'string', required: false },
    { name: 'authQueryParameters', display: 'Additional Auth Query Parameters', type: 'string', required: false },
    { name: 'accessToken', display: 'Access Token', type: 'password', required: false },
    { name: 'refreshToken', display: 'Refresh Token', type: 'password', required: false },
    { name: 'expiresIn', display: 'Expires In (s, from issuer)', type: 'number', required: false },
    { name: 'grantType', display: 'Grant Type', type: 'options', options: ['authorizationCode', 'clientCredentials', 'pkce'], required: false, default: 'authorizationCode' },
  ],
}
