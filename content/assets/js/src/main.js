import "@babel/polyfill";
import {createChart} from "./chart"
import {getStatsFx} from "./effects"
import {addChart} from './stores'
import { segmentedEvent } from "./segmented";
import { openCalendar, closeCalendar, updateRange, selectRange } from './calendar'

const systemRed = '255, 69, 58'
const systemBlue = '10, 132, 255'
const systemYellow = '255, 214, 10'

async function polling() {
  await getStatsFx()
}

window.onload = () => {
  window.response_metrics = []
  window.calendar = []
  window.calendar_ranges = {from: {}, to: {}}
  window.date_ranges = {from: null, to: null}
  window.custom_date_ranges = false
  window.segmented_minutes = 1

  let chartAVGRT = document.getElementById("chartAVGRT").getContext('2d');
  let chartRPM = document.getElementById("chartRPM").getContext('2d');
  let chartRSS = document.getElementById("chartRSS").getContext('2d');
  let chartERR = document.getElementById("chartERR").getContext('2d');
  chartAVGRT = createChart(chartAVGRT, systemRed, "ms", 0, 2, true)
  chartRPM = createChart(chartRPM, systemBlue, "RPS", 0, 1, false, 10)
  chartRSS = createChart(chartRSS, systemBlue, "RSS", 0, 1, false, 10)
  chartERR = createChart(chartERR, systemBlue, "ERRORS", 0, 1, false, 10)

  let segmentedControls = document.querySelectorAll(".segmented-control label")
  for (var i=0; i<segmentedControls.length; i++) {
    segmentedControls[i].addEventListener('click', function(e) {
      segmentedEvent(e.target)
    })
  }

  addChart(chartAVGRT)
  addChart(chartRPM)
  addChart(chartRSS)
  addChart(chartERR)
  polling()

  document.getElementsByClassName('calendar-close')[0].addEventListener('click', (e)=>{
    closeCalendar(e.currentTarget)
  })
  document.getElementsByClassName('filter')[0].addEventListener('click', (e)=>{
    openCalendar()
  })

  document.querySelectorAll(".date-box").forEach(e => {
    e.addEventListener('click', function() {
      selectRange(e)
    })
  })

  document.querySelectorAll(".time input").forEach(e => {
    e.addEventListener('input', function() {
      if (e.dataset.value === e.value) return
      e.dataset.value = e.value
      updateRange(e)
    })
  })

  setInterval(polling, 1000)
}
