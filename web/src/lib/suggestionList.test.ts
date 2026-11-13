import { describe, expect, it } from 'vitest'
import { findSuggestionCombobox, isSuggestionListOpen } from './suggestionList'

/**
 * A stand-in for an element: the helpers only use `closest` and
 * `getAttribute`, so no DOM is needed for the test.
 */
function element(closestResult: unknown): EventTarget {
  return {
    closest: () => closestResult,
  } as unknown as EventTarget
}

function combobox(expanded: string | null): unknown {
  return {
    getAttribute: (name: string) => (name === 'aria-expanded' ? expanded : null),
  }
}

describe('suggestionList', () => {
  it('finds the combobox the target belongs to', () => {
    const box = combobox('true')
    expect(findSuggestionCombobox(element(box))).toBe(box)
  })

  it('returns null for a target outside of a combobox', () => {
    expect(findSuggestionCombobox(element(null))).toBeNull()
    expect(findSuggestionCombobox(null)).toBeNull()
    expect(findSuggestionCombobox({} as EventTarget)).toBeNull()
  })

  it('detects an open suggestion list', () => {
    expect(isSuggestionListOpen(element(combobox('true')))).toBe(true)
  })

  it('ignores a closed suggestion list', () => {
    expect(isSuggestionListOpen(element(combobox('false')))).toBe(false)
    expect(isSuggestionListOpen(element(combobox(null)))).toBe(false)
    expect(isSuggestionListOpen(element(null))).toBe(false)
    expect(isSuggestionListOpen(null)).toBe(false)
  })
})
