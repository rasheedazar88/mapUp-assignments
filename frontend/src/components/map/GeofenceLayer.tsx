import { Polygon, Tooltip } from 'react-leaflet'
import type { Geofence } from '../../api/types'
import { CATEGORIES } from '../../constants'

interface Props {
  geofences: Geofence[]
  highlightId?: string
  onSelect?: (g: Geofence) => void
  interactive?: boolean
}

export function GeofenceLayer({ geofences, highlightId, onSelect, interactive = true }: Props) {
  return (
    <>
      {geofences.map((g) => {
        const color = CATEGORIES[g.category]?.color ?? '#64748b'
        const highlighted = g.id === highlightId
        return (
          <Polygon
            key={g.id}
            positions={g.coordinates}
            interactive={interactive}
            pathOptions={{
              color,
              weight: highlighted ? 4 : 2,
              fillOpacity: highlighted ? 0.3 : 0.15,
              dashArray: g.category === 'restricted_zone' ? '6 4' : undefined,
            }}
            eventHandlers={onSelect ? { click: () => onSelect(g) } : undefined}
          >
            {interactive && (
              <Tooltip sticky>
                <strong>{g.name}</strong>
                <br />
                {CATEGORIES[g.category]?.label}
              </Tooltip>
            )}
          </Polygon>
        )
      })}
    </>
  )
}
