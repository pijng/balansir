import { h, spec, node } from 'forest';
import { combine } from 'effector';

const TAG_COLORS = {
  "INFO":    "#ffffff",
  "NOTICE":  "#30d158ff",
  "WARNING": "#ffd60aff",
  "ERROR":   "#ff453aff",
  "FATAL":   "#bf5af2ff	"
}

const Log = ($timestamp, $tag, $text, scrollID, isRendered, $tags, $searchInput) => {
  h('div', () => {
    spec({
      attr: {class: "log"},
      visible: combine($tags, $tag, $text, $searchInput, (tags, tag, text, searchInput) => {
        const currentTag = tags.find(t => t.name === tag)
        const matchSearch = text.toLowerCase().includes(searchInput.toLowerCase().trim())
        return currentTag.checked && matchSearch
      })
    })

    h('div', {
      attr: {class: "column timestamp"},
      text: $timestamp.map(t => new Date(t).toLocaleString())
    })
    h('div', {
      attr: {class: "column tag"},
      style: {
        color: $tag.map(t => TAG_COLORS[t])
      },
      text: $tag
    })
    h('div', {
      attr: {class: "column text"},
      style: {
        color: $tag.map(t => TAG_COLORS[t])
      },
      text: $text
    })

    node(node => {
      if (!isRendered) {
        clearTimeout(scrollID)
        scrollID = setTimeout(() => {
          node.scrollIntoView()
          isRendered = true
        }, 100)
      }
    })

  })
}

export { Log };