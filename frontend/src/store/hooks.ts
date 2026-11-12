import { useDispatch, useSelector, useStore } from 'react-redux'
import type { AppDispatch, RootState, store } from './index'

// Typed versions of the react-redux hooks.
export const useAppDispatch = useDispatch.withTypes<AppDispatch>()
export const useAppSelector = useSelector.withTypes<RootState>()
export const useAppStore = useStore.withTypes<typeof store>()
