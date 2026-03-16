// API client for Ottie Channel configuration.

interface OttieTokenResponse {
  token: string
  ws_url: string
  enabled: boolean
}

interface OttieSetupResponse {
  token: string
  ws_url: string
  enabled: boolean
  changed: boolean
}

const BASE_URL = ""

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, options)
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function getOttieToken(): Promise<OttieTokenResponse> {
  return request<OttieTokenResponse>("/api/ottie/token")
}

export async function regenOttieToken(): Promise<OttieTokenResponse> {
  return request<OttieTokenResponse>("/api/ottie/token", { method: "POST" })
}

export async function setupOttie(): Promise<OttieSetupResponse> {
  return request<OttieSetupResponse>("/api/ottie/setup", { method: "POST" })
}

export type { OttieTokenResponse, OttieSetupResponse }
