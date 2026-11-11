import L from 'leaflet'
import { useMemo } from 'react'
import { Marker, Tooltip } from 'react-leaflet'
import type { VehicleStatus } from '../../api/types'
import { formatDateTime } from '../../utils/format'

// A divIcon is plain HTML, which avoids Leaflet's default image icons (they
// break under bundlers) and lets us color markers by status.
function vehicleIcon(color: string, selected: boolean) {
  return L.divIcon({
    className: '',
    html: `<div class="vehicle-marker${selected ? ' selected' : ''}" style="background:${color}">&#128666;</div>`,
    iconSize: [30, 30],
    iconAnchor: [15, 15],
  })
}

function statusColor(v: VehicleStatus): string {
  if (v.current_geofences.some((g) => g.category === 'restricted_zone')) return '#dc2626'
  if (v.current_geofences.length > 0) return '#2563eb'
  return '#475569'
}

interface Props {
  vehicles: VehicleStatus[]
  selectedId?: string
  onSelect?: (v: VehicleStatus) => void
}

export function VehicleMarkers({ vehicles, selectedId, onSelect }: Props) {
  const located = useMemo(() => vehicles.filter((v) => v.current_location), [vehicles])
  return (
    <>
      {located.map((v) => {
        const loc = v.current_location!
        return (
          <Marker
            key={v.vehicle_id}
            position={[loc.latitude, loc.longitude]}
            icon={vehicleIcon(statusColor(v), v.vehicle_id === selectedId)}
            zIndexOffset={v.vehicle_id === selectedId ? 1000 : 0}
            eventHandlers={onSelect ? { click: () => onSelect(v) } : undefined}
          >
            <Tooltip direction="top" offset={[0, -14]}>
              <strong>{v.vehicle_number}</strong> · {v.driver_name}
              <br />
              {v.current_geofences.length > 0
                ? `Inside: ${v.current_geofences.map((g) => g.geofence_name).join(', ')}`
                : 'Not inside any geofence'}
              <br />
              <span style={{ color: '#64748b' }}>{formatDateTime(loc.timestamp)}</span>
            </Tooltip>
          </Marker>
        )
      })}
    </>
  )
}
