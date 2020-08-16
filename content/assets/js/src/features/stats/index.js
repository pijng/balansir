import { createStore, createEvent, forward, sample } from 'effector';

const $stats = createStore([])

const addStat = createEvent()
$stats.on(addStat, (state, params) => [...state, params])

const setStats = createEvent()
forward({
  from: setStats,
  to: $stats
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

export { $stats, addStat, setStats, $responseTimes, $filteredStats };