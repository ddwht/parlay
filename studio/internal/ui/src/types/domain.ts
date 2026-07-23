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

export const BASE_FIELD_TYPES = [
  'string',
  'int',
  'float',
  'bool',
  'uuid',
  'timestamp',
  'bytes',
  'ref',
] as const;

export const TONES: Tone[] = ['neutral', 'info', 'warning', 'danger', 'success'];

/** Full field-type vocabulary: 8 base types followed by declared enum names. */
export function fieldTypeOptions(model: DomainModelDocument): string[] {
  return [...BASE_FIELD_TYPES, ...model.enums.map((e) => e.name)];
}
