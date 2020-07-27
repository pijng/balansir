import { h, spec, list } from 'forest';
import { Log } from './log';
import { Search } from './search';
import { TagFilter } from './tag-filter';

const LogsBlock = ($logs, openTagFilter, $isFilterVisible, focusSearch, $isSearchFocused, toggleTag, $tags, searchLogs, $searchInput, clearSearch) => {

  h('div', () => {
    spec({ attr: {class: "logs-wrapper"} })

    h('div', () => {
      spec({ attr: {class: "logs-header"} })
      
      TagFilter(openTagFilter, $isFilterVisible, toggleTag, $tags)
      Search(focusSearch, $isSearchFocused, searchLogs, clearSearch, $searchInput)
    })

    h('div', () => {
      spec({ attr: {class: "logs-table"} })

      let scrollID
      let isRendered = false
      list({
        source: $logs,
        fields: ['tag', 'timestamp', 'text'],
        fn({ fields: [$tag, $timestamp, $text] }) {
          Log($timestamp, $tag, $text, scrollID, isRendered, $tags, $searchInput)
        }
      })

    })

  })
}

export { LogsBlock };