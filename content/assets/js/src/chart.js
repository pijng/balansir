import { updateCalendar } from './calendar';
import { Chart } from 'chart.js'

export const createChart = (chart, color, label, pointRadius, borderWidth, ticks, chartPadding) => {
  var ctx = chart
  
  var gradient = ctx.createLinearGradient(0, 0, 0, 450);
  gradient.addColorStop(0, `rgba(${color}, 0.3)`);
  gradient.addColorStop(0.2, `rgba(${color}, 0.2)`);
  gradient.addColorStop(0.7, `rgba(${color}, 0.15)`);
  gradient.addColorStop(1, `rgba(${color}, 0.1)`);

  return new Chart(ctx, {
    type: 'line',
    data: {
      labels: [],
      datasets: [{
        label: label,
        borderColor: `rgba(${color}, 1)`,
        backgroundColor: gradient,
        pointBackgroundColor: `rgba(${color}, 1)`,
        pointRadius: pointRadius,
        borderWidth: borderWidth,
        fill: true,
        data: [],
      }]
    },
    options: {
      layout: {
        padding: {
            bottom: chartPadding,
            left: chartPadding,
        }
      },
      maintainAspectRatio: false,
      legend: {
        display: false
      },
      tooltips: {
        intersect: false,
        custom: function(tooltip) {
          if (!tooltip) return;
          tooltip.displayColors = false;
        },
      },
      elements: {
        line: {
          tension: 0,
        }
      },
      scales: {
        yAxes: [{
          gridLines: {
            color: 'rgba(235.0, 235.0, 245.0, 0.08)',
            lineWidth: 1
          },
          ticks: {
            fontColor: "#ebebf599",
            fontSize: 14,
            beginAtZero: true,
            display: ticks,
            fontFamily: "SF Pro Display Regular, 'SF Pro Display Regular', sans-serif",
          }
        }],
        xAxes: [{
          gridLines: {
            display: false,
          },
          ticks: {
            fontColor: "#ebebf599",
            fontSize: 14,
            display: ticks,
            autoSkip: true,
            maxTicksLimit: 8,
            maxRotation: 0,
            fontFamily: "SF Pro Display Regular, 'SF Pro Display Regular', sans-serif",
            callback: function(value) { 
              return formatDate(value); 
            },
          }
        }]
      },
    },
  })
}

const formatDate = (date) => {
  return `${(date.getHours()<10?'0':'')+date.getHours()}:${(date.getMinutes()<10?'0':'')+date.getMinutes()}:${(date.getSeconds()<10?'0':'')+date.getSeconds()}`
}

export const updateTimeRange = (chart, timeRange) => {
  if (window.date_ranges == timeRange) return
  if (timeRange.from !== null) {
    window.date_ranges.from = timeRange.from || window.date_ranges.from
  } else if (timeRange.from === undefined) {
  } else {
    window.date_ranges.from = null
  }
  if (timeRange.to !== null) {
    window.date_ranges.to = timeRange.to || window.date_ranges.to
  } else if (timeRange.to === undefined) {
  } else {
    window.date_ranges.to = null
  }

  redrawAVGRTchart(chart)
}

const redrawAVGRTchart = (chart) => {
  let tempTo = window.response_metrics[window.response_metrics.length-1].stamp
  let tempDate = new Date(tempTo)
  let tempFrom = window.response_metrics[0].stamp
  if (!window.custom_date_ranges) {
    tempFrom = tempDate.setMinutes(tempTo.getMinutes() - window.segmented_minutes)
  }
  let metrics = window.response_metrics.filter(v => v.stamp >= (window.date_ranges.from||tempFrom) && v.stamp <= (window.date_ranges.to||tempTo))
  if (metrics.length < 1) return
  let data = metrics.map(stats => stats.value)
  let labels = metrics.map(stats => stats.stamp)
  chart.data.labels = labels
  chart.data.datasets[0].data = data
  updateAVGRTLabels(chart)
  chart.update(0)
}

const updateAVGRTLabels = (chart) => {
  updateLabel(chart, document.querySelector(".chart_avgrt .value_avg p"), "", "ms")
  updatePercentile(chart, document.querySelector(".chart_avgrt .value_99_percentile p"), "ms", 99)
  updatePercentile(chart, document.querySelector(".chart_avgrt .value_90_percentile p"), "ms", 90)
}

const dateLimit = (newDate, lastDate) => {
  return Math.round((newDate-lastDate)/1000) >= window.segmented_minutes*60
}

const addData = (chart, data, date, cnst=false) => {
  let label = date
  chart.data.datasets.forEach((dataset) => {
    if (cnst) {
      chart.data.labels.push(label);
      dataset.data.push(data)
      if (dataset.data.length > 60) {
        dataset.data.shift()
        chart.data.labels.shift()
      }
      return
    }

    if (!window.custom_date_ranges) {
      if (dateLimit(date, chart.data.labels[0])) {
        if (chart.canvas.id === 'chartAVGRT') {
          redrawAVGRTchart(chart)
        } else {
          dataset.data.shift()
          chart.data.labels.shift()
        }
      }
    }
    if (!window.date_ranges.to || !window.custom_date_ranges) {
      chart.data.labels.push(label);
      dataset.data.push(data)
    }
  })
  chart.update(0)
}

const updateLabel = (chart, target, label, postfix) => {
  let acc = chart.data.datasets[0].data.reduce((acc, current) => acc + current)
  let avg = acc/chart.data.datasets[0].data.length
  target.innerText = `${label} ${avg.toFixed(0)} ${postfix||""}`
}

const getPercentileIndex = (data, percentile) => {
  return Math.round(percentile/100 * data.length)
}

const updatePercentile = (chart, target, postfix, percentile) => {
  let data = [...chart.data.datasets[0].data]
  data.sort((i, j) => (i < j ? -1 : 1))
  let index = getPercentileIndex(data, percentile)
  let value = data[index-1]
  target.innerText = `${value.toFixed(0)} ${postfix||""}`
}

export function updateCharts(charts, stats) {
  let date = new Date(stats.timestamp);
  charts.forEach((chart) => {
    switch (chart.canvas.id) {
      case "chartAVGRT":
        window.response_metrics.push({value: stats.average_response_time, stamp: date})
        updateCalendar()
        addData(chart, stats.average_response_time, date)
        updateAVGRTLabels(chart)
        break
      case "chartRPM":
        addData(chart, stats.requests_per_second, date, true)
        updateLabel(chart, document.querySelector(".chart_rpm p"), "")
        break
      case "chartRSS":
        addData(chart, stats.memory_usage, date, true)
        document.querySelector(".chart_rss p").innerText = `${stats.memory_usage} ${stats.memory_usage > 1000 ? "GB":"MB"}`
        break
      case "chartERR":
        addData(chart, stats.errors_count, date, true)
        let acc = chart.data.datasets[0].data.reduce((acc, current) => acc + current)
        document.querySelector(".chart_err p").innerText = `${acc}`
        break
      default:
        break
    }
  })
}