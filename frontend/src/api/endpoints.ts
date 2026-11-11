// One function per backend endpoint. Pages never build URLs themselves.
import { http } from './client'
import type {
  ConfigureAlertRequest,
  ConfigureAlertResponse,
  CreateGeofenceRequest,
  CreateGeofenceResponse,
  CreateVehicleRequest,
  CreateVehicleResponse,
  FiredAlertsResponse,
  Geofence,
  GeofenceCategory,
  HealthResponse,
  ListAlertsResponse,
  ListGeofencesResponse,
  ListVehicleLocationsResponse,
  ListVehiclesResponse,
  LocationUpdateRequest,
  LocationUpdateResponse,
  AlertRule,
  Timed,
  VehicleLocationResponse,
  ViolationHistoryResponse,
  ViolationQuery,
} from './types'

// Drop empty values so we never send "?vehicle_id=".
function clean<T extends object>(params: T): Partial<T> {
  return Object.fromEntries(Object.entries(params).filter(([, v]) => v !== undefined && v !== null && v !== '')) as Partial<T>
}

export const api = {
  // README endpoints
  createGeofence: (body: CreateGeofenceRequest) => http.post<CreateGeofenceResponse>('/geofences', body).then((r) => r.data),
  listGeofences: (category?: GeofenceCategory) =>
    http.get<ListGeofencesResponse>('/geofences', { params: clean({ category }) }).then((r) => r.data),
  createVehicle: (body: CreateVehicleRequest) => http.post<CreateVehicleResponse>('/vehicles', body).then((r) => r.data),
  listVehicles: () => http.get<ListVehiclesResponse>('/vehicles').then((r) => r.data),
  updateLocation: (body: LocationUpdateRequest) =>
    http.post<LocationUpdateResponse>('/vehicles/location', body).then((r) => r.data),
  getVehicleLocation: (vehicleId: string) =>
    http.get<VehicleLocationResponse>(`/vehicles/location/${encodeURIComponent(vehicleId)}`).then((r) => r.data),
  configureAlert: (body: ConfigureAlertRequest) =>
    http.post<ConfigureAlertResponse>('/alerts/configure', body).then((r) => r.data),
  listAlerts: (filter: { geofence_id?: string; vehicle_id?: string } = {}) =>
    http.get<ListAlertsResponse>('/alerts', { params: clean(filter) }).then((r) => r.data),
  violationHistory: (q: ViolationQuery) =>
    http.get<ViolationHistoryResponse>('/violations/history', { params: clean(q) }).then((r) => r.data),

  // Extras
  health: () => http.get<HealthResponse>('/health').then((r) => r.data),
  getGeofence: (id: string) => http.get<Geofence & Timed>(`/geofences/${encodeURIComponent(id)}`).then((r) => r.data),
  listVehicleLocations: () => http.get<ListVehicleLocationsResponse>('/vehicles/locations').then((r) => r.data),
  setAlertStatus: (alertId: string, status: 'active' | 'inactive') =>
    http.patch<AlertRule & Timed>(`/alerts/${encodeURIComponent(alertId)}`, { status }).then((r) => r.data),
  deleteAlert: (alertId: string) => http.delete<Timed>(`/alerts/${encodeURIComponent(alertId)}`).then((r) => r.data),
  firedAlerts: (limit = 50) => http.get<FiredAlertsResponse>('/alerts/events', { params: { limit } }).then((r) => r.data),
}
