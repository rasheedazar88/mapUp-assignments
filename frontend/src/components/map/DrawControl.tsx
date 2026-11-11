import '@geoman-io/leaflet-geoman-free'
import L from 'leaflet'
import { useEffect, useRef } from 'react'
import { useMap } from 'react-leaflet'
import type { LatLngTuple } from '../../api/types'
import { closeRing, round6 } from '../../utils/coords'

interface Props {
  /** Called with a CLOSED ring of [lat, lng] pairs whenever the shape is drawn or edited. */
  onChange: (coords: LatLngTuple[] | null) => void
  /** Change this value to remove the drawn shape (e.g. when the user types coordinates instead). */
  resetKey?: number
  color?: string
}

/**
 * Polygon drawing via Leaflet-Geoman. Geoman owns the drawn layer (so the user
 * can drag vertices); we only read its coordinates out on create/edit.
 * Only one polygon is kept: drawing a new one replaces the old one.
 */
export function DrawControl({ onChange, resetKey, color = '#2563eb' }: Props) {
  const map = useMap()
  const layerRef = useRef<L.Polygon | null>(null)
  const onChangeRef = useRef(onChange)
  useEffect(() => {
    onChangeRef.current = onChange
  })

  useEffect(() => {
    map.pm.addControls({
      position: 'topleft',
      drawPolygon: true,
      editMode: true,
      removalMode: true,
      dragMode: true,
      drawMarker: false,
      drawCircleMarker: false,
      drawPolyline: false,
      drawRectangle: false,
      drawCircle: false,
      drawText: false,
      cutPolygon: false,
      rotateMode: false,
    })
    map.pm.setPathOptions({ color, fillOpacity: 0.2 })
    map.pm.setGlobalOptions({ allowSelfIntersection: false })

    const emit = (layer: L.Polygon) => {
      // getLatLngs() of a simple polygon is [ring]; the ring is open (no closing point).
      const ring = (layer.getLatLngs() as L.LatLng[][])[0] ?? []
      onChangeRef.current(closeRing(ring.map((p) => [round6(p.lat), round6(p.lng)] as LatLngTuple)))
    }

    const handleCreate = (e: { layer: L.Layer }) => {
      if (layerRef.current) map.removeLayer(layerRef.current)
      const layer = e.layer as L.Polygon
      layerRef.current = layer
      layer.on('pm:edit pm:dragend', () => emit(layer))
      layer.on('pm:remove', () => {
        layerRef.current = null
        onChangeRef.current(null)
      })
      emit(layer)
    }

    map.on('pm:create', handleCreate)
    return () => {
      map.off('pm:create', handleCreate)
      map.pm.removeControls()
      map.pm.disableDraw()
      if (layerRef.current) {
        map.removeLayer(layerRef.current)
        layerRef.current = null
      }
    }
  }, [map, color])

  // External reset (user switched to typing coordinates).
  useEffect(() => {
    if (resetKey === undefined) return
    if (layerRef.current) {
      map.removeLayer(layerRef.current)
      layerRef.current = null
    }
  }, [resetKey, map])

  return null
}
