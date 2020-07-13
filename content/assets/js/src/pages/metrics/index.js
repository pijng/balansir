import { h, spec } from 'forest';
import { MajorChart, MinorChart, Navigation } from '../../ui';
import { selectRange, $inputs } from '../../features/segmented_control';
import { daySelected } from '../../features/chart';
import { selectTime, $times, $monthSelected } from '../../features/calendar';
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
  Navigation()
  h('div', () => {
    spec({ attr: {class: "stats-view"} })
  
    h('div', () => {
      spec({ attr: {class: "chart-inline-block"} })
  
      MinorChart($RPM, "RPM", "chartRPM")
      MinorChart($RSS, "MEMORY", "chartRSS")
      MinorChart(0, "ERRORS", "chartERR")
    })

    MajorChart($AVG, $99percentile, $90percentile, $uniqueDatesArray, selectRange, 
      $inputs, $selectedSpan, selectSpan, $isVisible, daySelected, $spans, selectTime,
      $times, $monthSelected
    )
  })
}

export { Metrics };