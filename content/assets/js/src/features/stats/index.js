import { createStore, createEvent, forward, sample } from 'effector';
import { getStatsFx, getCollectedStatsFx } from '../polling';

const $stats = createStore([])

forward({
  from: getCollectedStatsFx.doneData,
  to: $stats
})

const addStat = createEvent()
$stats.on(addStat, (state, params) => [...state, params])

forward({
  from: getStatsFx.doneData,
  to: addStat
})

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