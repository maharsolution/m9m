/**
 * Helpers around the suggestion list of the expression editor.
 *
 * The expression fields render a combobox (`role="combobox"`) whose
 * `aria-expanded` attribute tells whether the autocomplete list is
 * open. Keyboard handlers outside of the field (the node details view
 * closes itself with Escape) have to know about that state: while the
 * list is open, Escape belongs to the list and the panel stays open
 * (n8n parity).
 *
 * The helpers work on any object that behaves like an element, so they
 * can be unit tested without a DOM.
 */

/** The combobox that owns the suggestion list, if the target is inside one. */
export function findSuggestionCombobox(target: EventTarget | null): Element | null {
  if (!target) return null
  const element = target as Element
  if (typeof element.closest !== 'function') return null
  return element.closest('[role="combobox"]')
}

/** True while the suggestion list of the focused editor is open. */
export function isSuggestionListOpen(target: EventTarget | null): boolean {
  const combobox = findSuggestionCombobox(target)
  if (!combobox || typeof combobox.getAttribute !== 'function') return false
  return combobox.getAttribute('aria-expanded') === 'true'
}
