import type { GeofenceCategory, RuleEventType } from './api/types'

export const CATEGORIES: Record<GeofenceCategory, { label: string; color: string; tag: string }> = {
  delivery_zone: { label: 'Delivery zone', color: '#2563eb', tag: 'blue' },
  restricted_zone: { label: 'Restricted zone', color: '#dc2626', tag: 'red' },
  toll_zone: { label: 'Toll zone', color: '#d97706', tag: 'orange' },
  customer_area: { label: 'Customer area', color: '#16a34a', tag: 'green' },
}

export const CATEGORY_OPTIONS = (Object.keys(CATEGORIES) as GeofenceCategory[]).map((value) => ({
  value,
  label: CATEGORIES[value].label,
}))

export const RULE_EVENT_OPTIONS: { value: RuleEventType; label: string }[] = [
  { value: 'entry', label: 'Entry' },
  { value: 'exit', label: 'Exit' },
  { value: 'both', label: 'Entry & exit' },
]

export const VEHICLE_TYPES = ['truck', 'van', 'car', 'motorcycle', 'bus', 'other']
