export type Tone = 'neutral' | 'info' | 'warning' | 'danger' | 'success';

export interface DomainField {
  name: string;
  type: string;
  target?: string;
  enum?: string;
  required: boolean;
}

export interface DomainEntity {
  name: string;
  fields: DomainField[];
}

export interface DomainEnumValue {
  value: string;
  label?: string;
  tone?: Tone;
}

export interface DomainEnum {
  name: string;
  values: DomainEnumValue[];
}

export interface DomainRelationship {
  name: string;
  from: string;
  to: string;
  cardinality: string;
}

export interface DomainModelDocument {
  schema_version: number;
  enums: DomainEnum[];
  entities: DomainEntity[];
  relationships?: DomainRelationship[];
  operations?: unknown[];
}

/**
 * The closed scalar field-type set, mirroring domain-model.schema.md's
 * "Closed field-type set" table exactly.
 *
 * This list had drifted from the schema in both directions at once: it
 * offered `timestamp` and `bytes`, which are not in the closed set and fail
 * deep validation with field-type-outside-closed-set, and it omitted
 * `datetime`, which IS in the set and is the type real models use for
 * timestamps. So the picker presented two dead ends and withheld the one
 * option a designer adding a date field actually needed — there was no way to
 * author a datetime field through the UI at all.
 *
 * Ordered to match the schema's table so the two can be diffed by eye.
 */
export const BASE_FIELD_TYPES = [
  'uuid',
  'string',
  'int',
  'float',
  'bool',
  'datetime',
  'ref',
] as const;

export const TONES: Tone[] = ['neutral', 'info', 'warning', 'danger', 'success'];

/** Full field-type vocabulary: the closed scalar set followed by declared enum names. */
export function fieldTypeOptions(model: DomainModelDocument): string[] {
  return [...BASE_FIELD_TYPES, ...model.enums.map((e) => e.name)];
}
