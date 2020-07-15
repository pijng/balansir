import { using, h, spec } from 'forest';
import { initCharts } from './features/chart';
import { getStatsFx, getCollectedStatsFx } from './features/polling';
import { Metrics } from './pages/metrics';
import { openCalendar } from './features/calendar';

using(document.querySelector('body'), () => {

  h('section', () => {
    spec({
      attr: {class: "layout"},
      handler: {click: openCalendar.prepend(e => e)}
    })
    Metrics()
  })
  getCollectedStatsFx()
  setInterval(getStatsFx, 1000)
})

window.onload = () => {
  initCharts()
}