import { configureStore } from '@reduxjs/toolkit'
import alertReducer from './alertSlice'

export const store = configureStore({
  reducer: {
    alerts: alertReducer,
  },
})

export type RootState = ReturnType<typeof store.getState>
export type AppDispatch = typeof store.dispatch
