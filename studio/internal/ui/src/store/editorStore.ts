import { create } from 'zustand';
import type {
  DomainModelDocument,
  DomainEntity,
  DomainEnum,
  DomainField,
} from '../types/domain';
import { BASE_FIELD_TYPES } from '../types/domain';
import {
  loadModel,
  saveModel,
  ApiError,
  type FieldError,
  type ModelEnvelope,
} from '../lib/api';

// ---------------------------------------------------------------------------
// Pure model transforms & validation helpers (exported for reuse in components)
// ---------------------------------------------------------------------------

export function isBuiltinType(name: string): boolean {
  return (BASE_FIELD_TYPES as readonly string[]).includes(name);
}

/** Returns "Entity.field" of the first ref pointing at entityName, else null. */
export function findReferenceTo(
  model: DomainModelDocument,
  entityName: string,
): string | null {
  for (const entity of model.entities) {
    for (const field of entity.fields) {
      if (field.type === 'ref' && field.target === entityName) {
        return `${entity.name}.${field.name}`;
      }
    }
  }
  return null;
}

export function renameEntityInModel(
  model: DomainModelDocument,
  oldName: string,
  newName: string,
): DomainModelDocument {
  return {
    ...model,
    entities: model.entities.map((entity) => ({
      ...entity,
      name: entity.name === oldName ? newName : entity.name,
      fields: entity.fields.map((f) =>
        f.type === 'ref' && f.target === oldName ? { ...f, target: newName } : f,
      ),
    })),
    relationships: model.relationships?.map((rel) => ({
      ...rel,
      from: rel.from === oldName ? newName : rel.from,
      to: rel.to === oldName ? newName : rel.to,
    })),
  };
}

export function renameEnumInModel(
  model: DomainModelDocument,
  oldName: string,
  newName: string,
): DomainModelDocument {
  return {
    ...model,
    enums: model.enums.map((e) =>
      e.name === oldName ? { ...e, name: newName } : e,
    ),
    entities: model.entities.map((entity) => ({
      ...entity,
      fields: entity.fields.map((f) => {
        if (f.enum === oldName || f.type === oldName) {
          return { ...f, type: newName, enum: newName };
        }
        return f;
      }),
    })),
  };
}

/** Build a field-type change, keeping field.enum consistent with the choice. */
export function applyFieldType(
  field: DomainField,
  model: DomainModelDocument,
  nextType: string,
): DomainField {
  const isEnum = model.enums.some((e) => e.name === nextType);
  const next: DomainField = { ...field, type: nextType };
  if (isEnum) {
    next.enum = nextType;
    delete next.target;
  } else if (nextType === 'ref') {
    delete next.enum;
    // target chosen separately
  } else {
    delete next.enum;
    delete next.target;
  }
  return next;
}

/** Normalize an enum value so empty label/tone keys are never persisted. */
export function normalizeEnumValue(
  value: string,
  label?: string,
  tone?: string,
): DomainEnum['values'][number] {
  const out: DomainEnum['values'][number] = { value };
  if (label && label.trim() !== '') out.label = label;
  if (tone && tone !== '' && tone !== 'none') {
    out.tone = tone as DomainEnum['values'][number]['tone'];
  }
  return out;
}

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

export type Selection =
  | { kind: 'entity'; name: string }
  | { kind: 'enum'; name: string }
  | null;

export interface EditorState {
  model: DomainModelDocument;
  etag: string;
  isDirty: boolean;
  isSaving: boolean;
  isLoading: boolean;
  lastSavedAt: number | null;
  selection: Selection;

  // error / lifecycle surfaces
  fieldErrors: FieldError[];
  conflict: { currentEtag: string; attemptedEtag: string } | null;
  serverError: { requestId: string } | null;
  sessionEnded: boolean;
  hadActivity: boolean;

  // lifecycle
  resetStore: () => void;
  hydrate: (env: ModelEnvelope) => void;
  load: () => Promise<void>;

  // selection
  selectEntity: (name: string) => void;
  selectEnum: (name: string) => void;

  // model mutation
  applyModel: (next: DomainModelDocument) => void;

  // persistence
  save: () => Promise<ModelEnvelope | null>;
  reloadAndReapply: () => Promise<void>;
  keepDraft: () => void;
  dismissConflict: () => void;
  clearServerError: () => void;
}

function initialModel(): DomainModelDocument {
  return { schema_version: 1, enums: [], entities: [] };
}

export const useEditorStore = create<EditorState>((set, get) => ({
  model: initialModel(),
  etag: 'empty',
  isDirty: false,
  isSaving: false,
  isLoading: false,
  lastSavedAt: null,
  selection: null,
  fieldErrors: [],
  conflict: null,
  serverError: null,
  sessionEnded: false,
  hadActivity: false,

  resetStore: () =>
    set({
      model: initialModel(),
      etag: 'empty',
      isDirty: false,
      isSaving: false,
      isLoading: false,
      lastSavedAt: null,
      selection: null,
      fieldErrors: [],
      conflict: null,
      serverError: null,
      sessionEnded: false,
      hadActivity: false,
    }),

  hydrate: (env) =>
    set({
      model: env.model,
      etag: env.etag,
      isDirty: false,
      fieldErrors: [],
      conflict: null,
      serverError: null,
    }),

  load: async () => {
    set({ isLoading: true });
    try {
      const env = await loadModel();
      get().hydrate(env);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'network-error') {
        // initial load failure before any activity: not a session-ended case
      }
    } finally {
      set({ isLoading: false });
    }
  },

  selectEntity: (name) => set({ selection: { kind: 'entity', name } }),
  selectEnum: (name) => set({ selection: { kind: 'enum', name } }),

  applyModel: (next) =>
    set({ model: next, isDirty: true, hadActivity: true, fieldErrors: [] }),

  save: async () => {
    const { model, etag } = get();
    set({ isSaving: true, fieldErrors: [] });
    try {
      const env = await saveModel(model, etag);
      set({
        model: env.model,
        etag: env.etag,
        isDirty: false,
        isSaving: false,
        lastSavedAt: Date.now(),
      });
      return env;
    } catch (err) {
      set({ isSaving: false });
      if (err instanceof ApiError) {
        switch (err.code) {
          case 'conflict':
            set({
              conflict: {
                currentEtag: err.currentEtag ?? '',
                attemptedEtag: err.attemptedEtag ?? '',
              },
            });
            break;
          case 'validation-failed':
            set({ fieldErrors: err.fields ?? [] });
            break;
          case 'server-error':
            set({ serverError: { requestId: err.requestId ?? '' } });
            break;
          case 'network-error':
            if (get().hadActivity) set({ sessionEnded: true });
            break;
        }
      }
      return null;
    }
  },

  reloadAndReapply: async () => {
    // Refetch the authoritative model; adopt its etag. Never auto re-save.
    const env = await loadModel();
    set({
      model: env.model,
      etag: env.etag,
      conflict: null,
      isDirty: false,
    });
  },

  keepDraft: () => set({ conflict: null }),
  dismissConflict: () => set({ conflict: null }),
  clearServerError: () => set({ serverError: null }),
}));

// Convenience selectors ------------------------------------------------------

export function selectedEntity(state: EditorState): DomainEntity | null {
  if (state.selection?.kind !== 'entity') return null;
  return (
    state.model.entities.find((e) => e.name === state.selection!.name) ?? null
  );
}

export function selectedEnum(state: EditorState): DomainEnum | null {
  if (state.selection?.kind !== 'enum') return null;
  return state.model.enums.find((e) => e.name === state.selection!.name) ?? null;
}
