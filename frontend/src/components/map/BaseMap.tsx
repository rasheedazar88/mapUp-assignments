import L from 'leaflet'
import { useEffect, useRef, type ReactNode } from 'react'
import { MapContainer, TileLayer, useMap } from 'react-leaflet'
import type { LatLngTuple } from '../../api/types'
import { DEFAULT_CENTER, DEFAULT_ZOOM } from '../../config'

interface Props {
  children?: ReactNode
  className?: string
  /** Points the map should fit into view (geofence corners, vehicles…). */
  fitTo?: LatLngTuple[]
  /** Change this value to re-fit; by default the map fits only once, so it doesn't jump while the user pans. */
  fitKey?: string | number
}

/** OpenStreetMap base map. Leaflet uses [lat, lng], the same order as our API. */
export function BaseMap({ children, className = 'h-[480px]', fitTo, fitKey }: Props) {
  return (
    <MapContainer center={DEFAULT_CENTER} zoom={DEFAULT_ZOOM} className={className} scrollWheelZoom>
      <TileLayer
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
      />
      <FitBounds points={fitTo} fitKey={fitKey} />
      <InvalidateOnResize />
      {children}
    </MapContainer>
  )
}

function FitBounds({ points, fitKey }: { points?: LatLngTuple[]; fitKey?: string | number }) {
  const map = useMap()
  const fittedFor = useRef<string | number | undefined | null>(null)
  useEffect(() => {
    if (!points || points.length === 0) return
    if (fittedFor.current === (fitKey ?? 'once')) return
    fittedFor.current = fitKey ?? 'once'
    if (points.length === 1) {
      map.setView(points[0], Math.max(map.getZoom(), 15))
    } else {
      map.fitBounds(L.latLngBounds(points), { padding: [30, 30], maxZoom: 16 })
    }
  }, [map, points, fitKey])
  return null
}

/** Leaflet measures its container once; tell it when the container size changes (drawers, sidebar collapse). */
function InvalidateOnResize() {
  const map = useMap()
  useEffect(() => {
    const el = map.getContainer()
    const ro = new ResizeObserver(() => map.invalidateSize())
    ro.observe(el)
    return () => ro.disconnect()
  }, [map])
  return null
}
