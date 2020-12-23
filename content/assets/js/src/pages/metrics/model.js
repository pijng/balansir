import { sample } from 'effector';
import { $uniqueDatesArray, $selectedSpan, selectSpan, openCalendar, $isVisible } from '@features/calendar';
import { $spans } from '@features/calendar';
import { $responseTimes, $filteredStats } from '@features/stats';

const $AVG = sample({
  source: $responseTimes,
  fn: (state) => Math.round(state.reduce((acc, val) => acc + val)/state.length)
})

const $sortedResponseTimes = sample({
  source: $responseTimes,
  fn: (state) => state.sort((i, j) => (i < j ? -1 : 1))
})

const $99percentile = sample({
  source: $sortedResponseTimes,
  fn: (state) => Math.round(state[Math.round(99/100 * state.length) - 1])
})

const $90percentile = sample({
  source: $sortedResponseTimes,
  fn: (state) => Math.round(state[Math.round(90/100 * state.length) - 1])
})

const $RPM = sample({
  source: $filteredStats,
  fn: (state) => Math.round(state.reduce((acc, val) => acc + val.requests_per_second, 0)/state.length)
})

const $RSS = sample({
  source: $filteredStats,
  fn: (state) => state[state.length-1].memory_usage
})

export {
  $uniqueDatesArray,
  $AVG,
  $99percentile,
  $90percentile,
  $RPM,
  $RSS,
  $selectedSpan,
  selectSpan,
  openCalendar,
  $isVisible,
  $spans
};