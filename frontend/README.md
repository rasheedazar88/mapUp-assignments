# GeoFleet frontend

React 19 + TypeScript + Vite, with Ant Design 6 (components), Tailwind 4 (layout utilities), React-Leaflet + Leaflet-Geoman (maps and polygon drawing), Redux Toolkit + React Redux (alert state), Axios and React Router 7.

```bash
cp .env.example .env.local   # VITE_API_URL=http://localhost:8080
npm install
npm run dev                  # http://localhost:5173
npm run build                # type-check + production build → dist/
npx vitest run               # unit tests (coordinate helpers, alert slice)
npx oxlint                   # lint
```

## Structure

```
src/
  api/          types.ts (mirrors the README contracts), client.ts (axios + error normalisation), endpoints.ts
  hooks/        useAlertSocket (single WebSocket, backoff reconnect, backfill), useRequest/useInterval
  store/        Redux store (index.ts), alertSlice (+ tests: live feed, unread count, socket status), typed hooks
  components/   layout/AppLayout, map/{BaseMap,GeofenceLayer,VehicleMarkers,DrawControl,ClickToPick}, common/
  pages/        Dashboard, Geofences, Vehicles, Tracking, AlertRules, LiveAlerts, ViolationHistory
  utils/        coords.ts (+ tests), format.ts
```

Leaflet uses `[lat, lng]`, the same order as the API, so the frontend never swaps coordinates. Only the backend swaps them when it talks to PostGIS.
