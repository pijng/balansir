import { createEvent, createStore, sample, createEffect, merge } from 'effector';
import { updateCharts } from './update';
import { $stats } from '../stats';
import { $selectedRange, selectRange } from '../segmented_control';
import { $selectedSpan, $spans, $times } from '../calendar';

const addChart = createEvent()
const $charts = createStore([])
  .on(addChart, (state, params) => [...state, params])

const updateChartsFx = createEffect("updateChartsFx", {
  handler: updateCharts
})

sample({
  source: [$charts, $stats, $selectedRange, $spans],
  clock: merge([$stats, selectRange]),
  fn: ([charts, stats, range, spans]) => {
    const majorTo = spans.to.active ? spans.to.date : stats[stats.length-1].timestamp
    var majorFrom

    if (spans.from.active) {
      majorFrom = spans.from.date
    } else {
      if (spans.to.active) {
        majorFrom = stats[0].timestamp
      } else {
        majorFrom = majorTo - range
      }
    }

    const majorStats = stats.filter(a => {
      return (a.timestamp >= majorFrom && a.timestamp <= majorTo)
    })

    const minorTo = stats[stats.length-1].timestamp
    const minorFrom = minorTo - 60000
    const minorStats = stats.filter(a => {
      return (a.timestamp >= minorFrom && a.timestamp <= minorTo)
    })
    return {charts, majorStats, minorStats}
  },
  target: updateChartsFx
})

const daySelected = createEvent()

sample({
  source: [$selectedSpan, $spans, $times],
  clock: daySelected,
  fn: ([selectedSpan, spans, times], params) => {
    var hour, minutes
    if (times) {
      [hour, minutes] = times.split(':')
    } else {
      [hour, minutes] = ["", ""]
    }
    const date = new Date(params.year, params.month, params.day, hour, minutes)
    const unixTime = date.getTime()

    if (selectedSpan.from) {
      if (spans.from.active) {
        return {
          from: {active: false, date: null, time: ""},
          to: {...spans.to}
        }
      }
      return {
        from: {active: true, date: unixTime, time: times},
        to: {...spans.to}
      }
    }
    if (selectedSpan.to) {
      if (spans.to.active) {
        return {
          from: {...spans.from},
          to: {active: false, date: null, time: ""}
        }
      }
      return {
        from: {...spans.from},
        to: {active: true, date: unixTime, time: times}
      }
    }
  },
  target: $spans
})

sample({
  source: [$charts, $stats, $spans],
  clock: $spans,
  fn: ([charts, stats, spans]) => {
    const {from, to} = spans
    const majorStats = stats.filter(a => {
      return (a.timestamp >= from.date && a.timestamp <= (to.date || stats[stats.length-1].timestamp))
    })
    const chart = [charts.find(c => c.canvas.id === 'chartAVGRT')]
    return ({charts: chart, majorStats: majorStats})
  },
  target: updateChartsFx
})

export { addChart, daySelected };