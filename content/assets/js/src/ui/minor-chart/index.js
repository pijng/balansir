import { h, spec, node } from 'forest';

const MinorChart = ($value, label, chartId, initChart, unit="") => {
  h('div', () => {
    spec({ attr: {class: "chart-wrapper chart-small"} })

    h('div', () => {
      spec({ attr: {class: "chart-label"} })

      h('p', { text: [$value, ` ${unit}`] })

      h('span', { text: `${label}` })
    })

    h('div', () => {
      spec({ attr: {class: "chart-wrapper_area"} })

      h('canvas', () => {
        spec({ attr: {id: `${chartId}`} })

        node(node => {
          initChart(node)
        })
      })
    })

  })
}

export { MinorChart };