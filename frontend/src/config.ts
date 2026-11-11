// Runtime configuration, from Vite env variables (set at build time).

const rawApiUrl = (import.meta.env.VITE_API_URL as string | undefined) ?? 'http://localhost:8080'

export const API_URL = rawApiUrl.replace(/\/+$/, '')

// http://host → ws://host/ws/alerts ; https://host → wss://host/ws/alerts
export const WS_URL = API_URL.replace(/^http/, 'ws') + '/ws/alerts'

export const API_KEY = (import.meta.env.VITE_API_KEY as string | undefined) || undefined

// Map view used when there is nothing to fit to (the README's example area).
export const DEFAULT_CENTER: [number, number] = [37.7799, -122.4144]
export const DEFAULT_ZOOM = 13
