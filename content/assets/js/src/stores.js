import {createStore, createEvent, sample, guard} from 'effector';
import {getStatsFx} from './effects'
import {updateCharts, updateTimeRange} from './chart'
import {updateChartRange, updateChartRangeHourMinute} from './calendar'

const $stats = createStore({})
  .on(getStatsFx.doneData, (_, params) => params)

export const addChart = createEvent()
const $charts = createStore([])
  .on(addChart, (state, params) => [...state, params])

const sampledChartsUpdate = sample({
  source: $charts,
  clock: $stats,
  fn: (charts, stats) => ({charts, stats})
})

sampledChartsUpdate.watch(({charts, stats}) => {
  updateCharts(charts, stats)
})

export const selectDate = createEvent()
export const updateDate = createEvent()
const $calendarDates = createStore([])
  .on(selectDate, (_, params) => params)
  .on(updateDate, (_, params) => params)

const guardedcalendarDates = guard({
  source: $calendarDates,
  filter: (date) => Object.keys(date).length > 0
})

guardedcalendarDates.watch((date) => {
  if (Object.keys(date).length == 1) {
    window.calendar_ranges[date.range] = {range: date.range}
    updateChartRange(window.calendar_ranges[date.range])
    return
  }
  if (Object.keys(date).length == 2) {
    if (Object.keys(window.calendar_ranges[date.range]).length == 0) return
    window.calendar_ranges[date.range][Object.keys(date)[1]] = date[Object.keys(date)[1]]
    updateChartRangeHourMinute(date.range, window.calendar_ranges[date.range].hour||null, window.calendar_ranges[date.range].minute||null)
    return
  }
  window.calendar_ranges[date.range] = date
  updateChartRange(window.calendar_ranges[date.range])
})

export const switchTimeRange = createEvent()
const $timeRange = createStore({})
  .on(switchTimeRange, (_, params) => params)

const sampledTimeRangeUpdate = sample({
  source: $charts,
  clock: $timeRange,
  fn: (charts, timeRange) => ({charts, timeRange})
})

const guardedTimeRangeUpdate = guard({
  source: sampledTimeRangeUpdate,
  filter: ({charts, _}) => charts.length > 0
})

guardedTimeRangeUpdate.watch(({charts, timeRange}) => {
  let chart = charts.filter(chart => chart.canvas.id == "chartAVGRT")[0]
  updateTimeRange(chart, timeRange)
})
