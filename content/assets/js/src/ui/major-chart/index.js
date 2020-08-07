import { h, spec, node } from 'forest';
import { SegmentedControl } from '../segmented-control';
import { MajorLabel } from './major-label';
import { Calendar } from '../calendar';

const MajorChart = ($AVG, $99percentile, $90percentile, $uniqueDatesArray,
  selectRange, $inputs, $selectedSpan, selectSpan, $isVisible, daySelected,
  $spans, selectTime, $times, $monthSelected, initChart
  ) => {
  h('div', () => {
    spec({ attr: {class: "wrapper chart-large"} })

    h('div', () => {
      spec({ attr: {class: "chart-wrapper chart_avgrt"} })

      h('div', () => {
        spec({ attr: {class: "date-filter"} })

        SegmentedControl(selectRange, $inputs, $spans)
        Calendar($uniqueDatesArray, $selectedSpan, selectSpan, $isVisible, daySelected,
          $spans, selectTime, $times, $monthSelected
        )
      })

      h('div', () => {
        spec({ attr: {class: "label-wrapper"} })

        MajorLabel($AVG, "value_avg", "AVG Response Time")
        MajorLabel($99percentile, "value_99_percentile", "99th percentile")
        MajorLabel($90percentile, "value_90_percentile", "90th percentile")
      })

      h('div', () => {
        spec({ attr: {class: "chart-wrapper_area"} })

        h('canvas', () => {
          spec( {attr: {id: "chartAVGRT"} })

          node(node => {
            initChart(node)
          })
        })
      })

    })

  })
}

export { MajorChart };