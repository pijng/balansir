import { Chart } from 'chart.js';
import { addChart } from './model';

const systemRed = '255, 69, 58'
const systemBlue = '10, 132, 255'
const systemYellow = '255, 214, 10'

const chartsMeta = [
  {id: "chartAVGRT", c: systemRed, l: "MS", r: 0, bw: 1, tx: true, p: 0}, 
  {id: "chartRPM", c: systemBlue, l: "RPS", r: 0, bw: 1, tx: false,  p: 0},
  {id: "chartRSS", c: systemBlue, l: "RSS", r: 0, bw: 1, tx: false,  p: 0},
  {id: "chartERR", c: systemBlue, l: "ERRS", r: 0, bw: 1, tx: false,  p: 0},
]

const chartFabrik = (chart, color, label, pointRadius, borderWidth, ticks, chartPadding) => {
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
        callbacks: {
          title: function(tooltipItem) {
            return formatDate(new Date(tooltipItem[0].label))
          }
        }
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

const createChart = ({id, c, l, r, bw, tx, p}) => {
  const node = document.getElementById(id).getContext('2d');
  const chart = chartFabrik(node, c, l, r, bw, tx, p)
  addChart(chart)
}

const initCharts = () => {
  for (const meta of chartsMeta) {
    createChart(meta)
  }
}

export { initCharts };