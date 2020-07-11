import { createStore, createEvent, forward, sample } from 'effector';
import { getStatsFx } from '../polling';

const addStat = createEvent()

forward({
  from: getStatsFx.doneData,
  to: addStat
})

const $stats = createStore([])
  .on(addStat, (state, params) => [...state, params])

const $filteredStats = createStore([{
  average_response_time: 0, 
  requests_per_second: 0,
  memory_usage: 0,
  errors_count: 0
}])

sample({
  source: $stats,
  clock: addStat,
  fn: (state) => {
    const to = state[state.length-1].timestamp
    const from = to - 60000
    return state.filter(a => {
      return (a.timestamp >= from && a.timestamp <= to)
    })
  },
  target: $filteredStats
})

const $responseTimes = sample({
  source: $filteredStats,
  fn: (state) => state.map(s => s.average_response_time)
})

export { $stats, addStat, $responseTimes, $filteredStats };