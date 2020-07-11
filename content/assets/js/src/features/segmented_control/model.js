import { createEvent, createStore } from 'effector';

const selectRange = createEvent()
const $selectedRange = createStore(60000)
  .on(selectRange, (_, params) => params)

const $inputs = createStore([
  {label: "1m", range: 60000, checked: true},
  {label: "5m", range: 300000, checked: false},
  {label: "30m", range: 1800000, checked: false},
  {label: "3h", range: 10800000, checked: false},
  {label: "24h", range: 86400000, checked: false}
])

$inputs.on(selectRange, (inputs, params) => {
  return inputs.map(input => {
    return {...input, checked: input.range === params}
  })
})

export { selectRange, $selectedRange, $inputs };