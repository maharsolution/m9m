import type {
  NodeProperty,
  NodePropertyOption,
  NodePropertyType,
  NodePropertyTypeOptions,
} from '@/types/node'

/**
 * Helpers for the collection renderer.
 *
 * m9m's `collection` / `fixedCollection` properties describe a set of
 * repeating rows. Two shapes are supported:
 *
 *  1. **Metadata-declared fields** — the property's options carry a
 *     `values` array (n8n's `INodePropertyCollection.values`). Every
 *     declared value becomes a control in the row.
 *  2. **Inferred fields** — the metadata does not describe the row, but
 *     the stored value does. The keys of the first stored row are used
 *     as the field names, which keeps existing workflows editable
 *     (e.g. the IF node's `leftValue` / `operator` / `rightValue`).
 *
 * When neither is available the renderer falls back to a generic
 * `name` / `value` pair.
 */

export interface CollectionFieldDef {
  name: string
  displayName: string
  type: NodePropertyType
  default?: unknown
  required?: boolean
  description?: string
  placeholder?: string
  options?: NodePropertyOption[]
  typeOptions?: NodePropertyTypeOptions
}

/** The property types the renderer understands. */
const KNOWN_PROPERTY_TYPES: NodePropertyType[] = [
  'string',
  'number',
  'boolean',
  'options',
  'multiOptions',
  'collection',
  'fixedCollection',
  'json',
  'code',
  'dateTime',
  'color',
  'resourceLocator',
]

/** Narrow a free-form type name to a known property type. */
export function toPropertyType(value: string | undefined): NodePropertyType {
  if (value && (KNOWN_PROPERTY_TYPES as string[]).includes(value)) {
    return value as NodePropertyType
  }
  return 'string'
}

export function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** The stored rows of a collection value. */
export function toRows(value: unknown): Record<string, unknown>[] {
  if (Array.isArray(value)) {
    return value.filter((entry): entry is Record<string, unknown> => isPlainObject(entry))
  }
  if (isPlainObject(value)) {
    // n8n wraps a single collection bucket in an object keyed by the
    // option name: `{ values: { …fields } }`.
    const keys = Object.keys(value)
    if (keys.length === 1 && isPlainObject(value[keys[0]])) {
      return [value[keys[0]] as Record<string, unknown>]
    }
    return [value]
  }
  return []
}

/** True when the collection stores its rows as a list. */
export function isListCollection(property: NodeProperty, value: unknown): boolean {
  if (Array.isArray(value)) return true
  return property.typeOptions?.multipleValues === true
}

function toFieldDef(option: NodePropertyOption): CollectionFieldDef {
  return {
    name: option.name,
    displayName: option.displayName ?? option.name,
    type: toPropertyType(option.type),
    default: option.default,
    required: option.required,
    description: option.description,
    placeholder: option.placeholder,
    options: option.options,
    typeOptions: option.typeOptions,
  }
}

/** Field definitions declared by the metadata (`options[].values`). */
export function declaredFieldDefs(property: NodeProperty): CollectionFieldDef[] {
  const options = property.options ?? []
  const bucket = options.find((option) => Array.isArray(option.values) && option.values.length)
  if (!bucket?.values) return []
  return bucket.values.map((child) => toFieldDef(child))
}

/** Field definitions inferred from a stored row. */
export function inferredFieldDefs(row: Record<string, unknown> | undefined): CollectionFieldDef[] {
  if (!row) return []
  return Object.keys(row).map((key) => {
    const value = row[key]
    let type: NodePropertyType = 'string'
    if (typeof value === 'number') type = 'number'
    else if (typeof value === 'boolean') type = 'boolean'
    else if (Array.isArray(value)) type = 'multiOptions'
    else if (isPlainObject(value)) type = 'json'
    return {
      name: key,
      displayName: key,
      type,
    }
  })
}

/**
 * The field definitions a row of this collection is rendered with.
 * Metadata wins over inference, inference wins over the generic pair.
 */
export function collectionFieldDefs(
  property: NodeProperty,
  row: Record<string, unknown> | undefined,
): CollectionFieldDef[] {
  const declared = declaredFieldDefs(property)
  if (declared.length) return declared
  const inferred = inferredFieldDefs(row)
  if (inferred.length) return inferred
  return [
    { name: 'name', displayName: 'Name', type: 'string' },
    { name: 'value', displayName: 'Value', type: 'string' },
  ]
}

/** A new, empty row for the given field definitions. */
export function defaultRow(defs: CollectionFieldDef[]): Record<string, unknown> {
  const row: Record<string, unknown> = {}
  for (const def of defs) {
    if (def.default !== undefined) {
      row[def.name] = def.default
      continue
    }
    switch (def.type) {
      case 'number':
        row[def.name] = 0
        break
      case 'boolean':
        row[def.name] = false
        break
      case 'multiOptions':
        row[def.name] = []
        break
      default:
        row[def.name] = ''
    }
  }
  return row
}
