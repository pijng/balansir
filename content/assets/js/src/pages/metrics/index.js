import { h, spec } from 'forest';
import { MajorChart, MinorChart, Navigation } from '@ui/index';
import { selectRange, $inputs } from '@features/segmented_control';
import { daySelected, initChart } from '@features/chart';
import { selectTime, $times, $monthSelected } from '@features/calendar';
import {
  $uniqueDatesArray,
  $AVG,
  $99percentile,
  $90percentile,
  $RPM,
  $RSS,
  $selectedSpan,
  selectSpan,
  $isVisible,
  $spans
} from './model';

const Metrics = () => {
  Navigation(document.location.pathname)
  h('div', () => {
    spec({ attr: {class: "container-view"} })

    h('div', () => {
      spec({ attr: {class: "chart-inline-block"} })

      MinorChart($RPM, "RPM", "chartRPM", initChart)
      MinorChart($RSS, "MEMORY", "chartRSS", initChart, 'MB')
      MinorChart("\u00A0", "CODES", "chartCODES", initChart)
    })

    MajorChart($AVG, $99percentile, $90percentile, $uniqueDatesArray, selectRange,
      $inputs, $selectedSpan, selectSpan, $isVisible, daySelected, $spans, selectTime,
      $times, $monthSelected, initChart
    )
  })
}

export { Metrics };