import { switchTimeRange } from './stores'

export const segmentedEvent = (e) => {
  switch (e.getAttribute("for")) {
    case "1m":
      window.segmented_minutes = 1
      break
    case "5m":
      window.segmented_minutes = 5
      break
    case "30m":
      window.segmented_minutes = 30
      break
    case "3h":
      window.segmented_minutes = 180
      break
    case "24h":
      window.segmented_minutes = 1440
      break
  }

  if (window.custom_date_ranges) {
    window.date_ranges = {from: null, to: null}
    window.custom_date_ranges = false
    window.calendar_ranges = {from: {}, to: {}}
    let filter = document.querySelector('.date-picker .filter')
    filter.querySelectorAll('.filter-date').forEach(v => v.innerText = '')
    filter.querySelector('.filter-label').classList.remove('altered')
    filter.querySelector('.filter-label').innerText = 'Filter date'
    document.querySelectorAll(".time input").forEach(v => v.value = '')
    let _selectedCell = document.querySelector('.cell.selected')
    if (_selectedCell) {
      _selectedCell.classList.remove('selected')
    }
    document.querySelectorAll('.segmented-control label').forEach(v => v.classList.add('available'))
  }
  resetTimeRangeListener()
}


const resetTimeRangeListener = () => {
  switchTimeRange({
    from: undefined,
    to: undefined
  })
}