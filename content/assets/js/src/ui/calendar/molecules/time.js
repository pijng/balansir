import { h, spec } from 'forest';

const Time = (selectTime, $times) => {
  h('div', () => {
    spec({ attr: {class: "time"} })

    h('input', {
      attr: {
        placeholder: "time",
        value: $times,
        type: 'time',
        required: true,
      },
      handler: {
        input: selectTime.prepend(e => e.target.value)
      }
    })
  })
}

export { Time };