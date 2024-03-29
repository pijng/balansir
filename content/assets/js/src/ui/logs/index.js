import { h, spec, list } from 'forest';
import { Log } from './log';
import { Search } from './search';
import { TagFilter } from './tag-filter';
import { Bar } from './bar';
import { getLocale } from '@lib/locale';
import { DATEFORMAT_OPTIONS } from '@lib/dates';

const LogsBlock = ($logs, openTagFilter, $isFilterVisible, focusSearch, $isSearchFocused, toggleTag, $tags, searchLogs, $searchInput, clearSearch) => {

  h('div', () => {
    spec({ attr: {class: "logs-wrapper"} })

    h('div', () => {
      spec({ attr: {class: "logs-header"} })

      TagFilter(openTagFilter, $isFilterVisible, toggleTag, $tags)
      Search(focusSearch, $isSearchFocused, searchLogs, clearSearch, $searchInput)
      Bar()
    })

    h('div', () => {
      spec({ attr: {class: "logs-table"} })

      let scrollID
      let isRendered = false
      const intl = new Intl.DateTimeFormat(getLocale(), DATEFORMAT_OPTIONS)
      list({
        source: $logs,
        fields: ['tag', 'timestamp', 'text'],
        fn({ fields: [$tag, $timestamp, $text] }) {
          Log($timestamp, $tag, $text, intl, scrollID, isRendered, $tags, $searchInput)
        }
      })

      h('div', {
        attr: {id: "anchor"}
      })

    })

  })
}

export { LogsBlock };