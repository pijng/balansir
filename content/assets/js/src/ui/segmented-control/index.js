import { createEvent, sample, combine } from 'effector';
import { h, list, remap, spec } from 'forest';

const SegmentedControl = (selectRange, $inputs, $spans) => {
  h('div', () => {
    spec({ attr: {class: "segmented-control", id: "segmented-control"} })

    list($inputs, ({store}) => {
      const [label, range, checked] = remap(store, ['label', 'range', 'checked'])
      const $selected = $spans.map(
        (spans) => !(spans.from.active || spans.to.active)
      )

      const clickHandler = createEvent()
      sample({
        source: range,
        clock: clickHandler,
        target: selectRange
      })

      h("input", {
        attr: { name: "range", type: "radio", checked: checked }
      })

      h("label", {
        text: label,
        attr: { class: $selected.map(s => s ? "available" : "disabled") },
        handler: { click: clickHandler }
      })
    })

  })
}

export { SegmentedControl };