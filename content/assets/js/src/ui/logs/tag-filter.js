import { h, spec, list } from 'forest';
import { sample, createEvent } from 'effector';

const TagFilter = (openTagFilter, $isFilterVisible, toggleTag, $tags) => {
  h('div', () => {
    spec({
      attr: {class: "filter"},
      handler: {click: openTagFilter.prepend(e => e)}
    })

    h('div', {
      attr: {class: "filter-label"},
      text: "Filter tags",
    })

    h('div', () => {
      spec({
        attr: {class: "tags-filter"},
        visible: $isFilterVisible,
      })

      list({
        source: $tags,
        fields: ['name', 'checked'],
        fn({ fields: [$name, $checked] }) {
          const tagClick = createEvent()
          sample({
            source: $name,
            clock: tagClick,
            fn: params => params,
            target: toggleTag
          })

          h('label', () => {
            h('input', {
              attr: {
                type: "checkbox",
                checked: $checked,
              },
              handler: {click: tagClick}
            })

            spec({
              text: $name,
              attr: {class: 'tag'}
            })
          })
        }
      })

    })

  })
}

export { TagFilter };