import { h, spec, node } from 'forest';
import { combine } from 'effector';

const TAG_COLORS = {
  "INFO":    "#ffffff",
  "NOTICE":  "#30d158ff",
  "WARNING": "#ffd60aff",
  "ERROR":   "#ff453aff",
  "FATAL":   "#bf5af2ff"
}

const Log = ($timestamp, $tag, $text, intl, scrollID, isRendered, $tags, $searchInput) => {
  h('div', () => {
    // Normalize timestamp outside of spec and/or node to prevent recomputing
    const $normalizedTimestamp = $timestamp.map(t => intl.format(new Date(t)))

    spec({
      attr: {class: "log"},
      visible: combine($tags, $normalizedTimestamp, $tag, $text, $searchInput, (tags, timestamp, tag, text, searchInput) => {
        const currentTag = tags.find(t => t.name === tag)
        const matchText = text.toLowerCase().includes(searchInput.toLowerCase().trim())
        const matchTimestamp = timestamp.toLowerCase().includes(searchInput.toLowerCase().trim())
        return currentTag.checked && (matchText || matchTimestamp)
      })
    })

    h('div', {
      attr: {class: "column timestamp"},
      text: $normalizedTimestamp
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
          node.scrollIntoView({behavior: 'smooth'})
          isRendered = true
        }, 200)
      }
    })

  })
}

export { Log };