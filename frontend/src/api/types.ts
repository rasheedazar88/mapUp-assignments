// Types mirror the README's request/response shapes exactly.
// Coordinates are ALWAYS [latitude, longitude], the same order Leaflet uses.

export type LatLngTuple = [number, number]

export type GeofenceCategory = 'delivery_zone' | 'restricted_zone' | 'toll_zone' | 'customer_area'
export type EventType = 'entry' | 'exit'
export type RuleEventType = EventType | 'both'

/** Every API response carries the server-side execution time in nanoseconds. */
export interface Timed {
  time_ns: string
}

// ---- Geofences ----
export interface CreateGeofenceRequest {
  name: string
  description?: string
  coordinates: LatLngTuple[]
  category: GeofenceCategory
}
export interface CreateGeofenceResponse extends Timed {
  id: string
  name: string
  status: string
}
export interface Geofence {
  id: string
  name: string
  description: string
  coordinates: LatLngTuple[]
  category: GeofenceCategory
  status?: string
  created_at: string
}
export interface ListGeofencesResponse extends Timed {
  geofences: Geofence[]
}

// ---- Vehicles ----
export interface CreateVehicleRequest {
  vehicle_number: string
  driver_name: string
  vehicle_type: string
  phone: string
}
export interface CreateVehicleResponse extends Timed {
  id: string
  vehicle_number: string
  status: string
}
export interface Vehicle {
  id: string
  vehicle_number: string
  driver_name: string
  vehicle_type: string
  phone: string
  status: string
  created_at: string
}
export interface ListVehiclesResponse extends Timed {
  vehicles: Vehicle[]
}

// ---- Locations ----
export interface LocationUpdateRequest {
  vehicle_id: string
  latitude: number
  longitude: number
  timestamp: string
}
export interface LocationUpdateResponse extends Timed {
  vehicle_id: string
  location_updated: boolean
  current_geofences: { geofence_id: string; geofence_name: string; status: 'inside' }[]
}
export interface GeofenceRef {
  geofence_id: string
  geofence_name: string
  category: GeofenceCategory
}
export interface CurrentLocation {
  latitude: number
  longitude: number
  timestamp: string
}
export interface VehicleLocationResponse extends Timed {
  vehicle_id: string
  vehicle_number: string
  current_location: CurrentLocation | null
  current_geofences: GeofenceRef[]
}
/** GET /vehicles/locations (extra bulk endpoint) */
export interface VehicleStatus {
  vehicle_id: string
  vehicle_number: string
  driver_name: string
  vehicle_type: string
  status: string
  current_location: CurrentLocation | null
  current_geofences: GeofenceRef[]
}
export interface ListVehicleLocationsResponse extends Timed {
  vehicles: VehicleStatus[]
}

// ---- Alert rules ----
export interface ConfigureAlertRequest {
  geofence_id: string
  vehicle_id?: string
  event_type: RuleEventType
}
export interface ConfigureAlertResponse extends Timed {
  alert_id: string
  geofence_id: string
  vehicle_id: string | null
  event_type: RuleEventType
  status: string
}
export interface AlertRule {
  alert_id: string
  geofence_id: string
  geofence_name: string
  vehicle_id: string | null
  vehicle_number: string | null
  event_type: RuleEventType
  status: 'active' | 'inactive'
  created_at: string
}
export interface ListAlertsResponse extends Timed {
  alerts: AlertRule[]
}

// ---- Real-time alert (WebSocket message format from the README) ----
export interface AlertMessage {
  event_id: string
  event_type: EventType
  timestamp: string
  vehicle: { vehicle_id: string; vehicle_number: string; driver_name: string }
  geofence: { geofence_id: string; geofence_name: string; category: GeofenceCategory }
  location: { latitude: number; longitude: number }
}
export interface FiredAlertsResponse extends Timed {
  events: AlertMessage[]
}

// ---- Violation history ----
export interface Violation {
  id: string
  vehicle_id: string
  vehicle_number: string
  geofence_id: string
  geofence_name: string
  category?: GeofenceCategory
  event_type: EventType
  latitude: number
  longitude: number
  timestamp: string
}
export interface ViolationQuery {
  vehicle_id?: string
  geofence_id?: string
  start_date?: string
  end_date?: string
  limit?: number
  offset?: number
}
export interface ViolationHistoryResponse extends Timed {
  violations: Violation[]
  total_count: number
}

export interface HealthResponse extends Timed {
  status: string
  database: string
  ws_clients: number
}
