const LINE_CHARTS = ["chartAVGRT", "chartRPM", "chartRSS"]

const updateCharts = ({charts, majorStats, minorStats}) => {
  for (const chart of charts) {
    var spec;

    switch (chart.canvas.id) {
      case "chartAVGRT":
        spec = majorStats.map(s => { return {value: s.average_response_time, timestamp: s.timestamp} })
        break;
      case "chartRPM":
        spec = minorStats.map(s => { return {value: s.requests_per_second, timestamp: s.timestamp} })
        break;
      case "chartRSS":
        spec = minorStats.map(s => { return {value: s.memory_usage, timestamp: s.timestamp} })
        break;
      case "chartCODES":
        spec = {value: majorStats[majorStats.length-1].status_codes}
        break;
    }
    update(chart, spec)
  }
}

const update = (chart, data) => {
  if (LINE_CHARTS.includes(chart.canvas.id)) {
    updateLineChart(chart, data)
  } else {
    updateBarChart(chart, data)
  }
}

const updateLineChart = (chart, data) => {
  const values = data.map(d => d.value)
  const labels = data.map(d => new Date(d.timestamp))

  chart.data.datasets[0].data = values
  chart.data.labels = labels

  chart.update(0)
}

const updateBarChart = (chart, data) => {
  const values = Object.values(data.value)
  const labels = Object.keys(data.value)

  const currentValues = JSON.stringify(chart.data.datasets[0].data)
  const currentLabels = JSON.stringify(chart.data.labels)

  const newValues = JSON.stringify(values)
  const newLabels = JSON.stringify(labels)

  if (currentValues !== newValues || currentLabels !== newLabels) {
    chart.data.datasets[0].data = values
    chart.data.labels = labels

    chart.update(0)
  }
}

export { updateCharts };