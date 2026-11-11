import axios, { AxiosError } from 'axios'
import { API_KEY, API_URL } from '../config'

export const http = axios.create({
  baseURL: API_URL,
  timeout: 15_000,
  headers: { 'Content-Type': 'application/json', ...(API_KEY ? { 'X-API-Key': API_KEY } : {}) },
})

/** Normalized error thrown by every API call. `fields` maps request field → problem. */
export class ApiError extends Error {
  status: number
  code: string
  fields: Record<string, string>

  constructor(message: string, status: number, code: string, fields: Record<string, string> = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.fields = fields
  }
}

interface ErrorBody {
  error?: { code?: string; message?: string; details?: { fields?: Record<string, string> } }
}

// Turn every failure (server error JSON, timeout, backend down) into an ApiError
// with a message that can be shown to the user as-is.
http.interceptors.response.use(
  (res) => res,
  (err: AxiosError<ErrorBody>) => {
    if (err.response) {
      const body = err.response.data?.error
      return Promise.reject(
        new ApiError(
          body?.message ?? `Request failed (${err.response.status})`,
          err.response.status,
          body?.code ?? 'http_error',
          body?.details?.fields ?? {},
        ),
      )
    }
    if (err.code === 'ECONNABORTED') {
      return Promise.reject(new ApiError('The server took too long to respond. Please try again.', 0, 'timeout'))
    }
    return Promise.reject(new ApiError(`Cannot reach the backend at ${API_URL}. Is it running?`, 0, 'network_error'))
  },
)

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message
  return 'Something went wrong'
}
