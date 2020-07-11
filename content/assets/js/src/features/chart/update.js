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
      case "chartERR":
        spec = minorStats.map(s => { return {value: s.errors_count, timestamp: s.timestamp} })
        break;
    }
    update(chart, spec)
  }
}

const update = (chart, data) => {
  const values = data.map(d => d.value)
  const labels = data.map(d => new Date(d.timestamp))

  chart.data.datasets[0].data = values
  chart.data.labels = labels

  chart.update(0)
}

export { updateCharts };