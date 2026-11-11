import L from 'leaflet'
import { Marker, useMapEvents } from 'react-leaflet'
import type { LatLngTuple } from '../../api/types'
import { round6 } from '../../utils/coords'

const pickIcon = L.divIcon({ className: '', html: '<div class="pick-marker"></div>', iconSize: [16, 16], iconAnchor: [8, 8] })

/** Click anywhere on the map to choose a point; shows a marker at the chosen point. */
export function ClickToPick({ value, onPick }: { value?: LatLngTuple | null; onPick: (p: LatLngTuple) => void }) {
  useMapEvents({
    click: (e) => onPick([round6(e.latlng.lat), round6(e.latlng.lng)]),
  })
  return value ? <Marker position={value} icon={pickIcon} interactive={false} /> : null
}
