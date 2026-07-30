// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
// parlay-extends: domain-model-editor/domain-model-editor-relationships/cross-cutting/relationships-editor-integration
import { create } from 'zustand';
import type {
  DomainModelDocument,
  DomainEntity,
  DomainEnum,
  DomainField,
  DomainRelationship,
} from '../types/domain';
import { BASE_FIELD_TYPES } from '../types/domain';
import {
  loadModel,
  saveModel,
  validateModel,
  ApiError,
  WHOLE_MODEL_PATH,
  type FieldError,
  type Finding,
  type ModelEnvelope,
} from '../lib/api';

// ---------------------------------------------------------------------------
// Relationship vocabulary & name-prefill helpers
// ---------------------------------------------------------------------------

/** The closed cardinality set — offered verbatim by the cardinality picker. */
export const CARDINALITIES = [
  'one-to-one',
  'one-to-many',
  'many-to-one',
  'many-to-many',
] as const;

export type Cardinality = (typeof CARDINALITIES)[number];

/** Naive English pluralization, enough for the name-prefill convenience. */
export function pluralize(word: string): string {
  if (/[^aeiou]y$/i.test(word)) return `${word.slice(0, -1)}ies`;
  if (/(s|x|z|ch|sh)$/i.test(word)) return `${word}es`;
  return `${word}s`;
}

/**
 * The convenience name a relationship pre-fills to from its endpoints:
 * `<from>-<plural(to)>`, lowercased (e.g. Customer → Order ⇒ customer-orders).
 * Empty until both endpoints are chosen. Once a name is manually edited the
 * caller stops regenerating it — the prefill is a convenience, not a constraint.
 */
export function relationshipNamePrefill(from: string, to: string): string {
  if (!from || !to) return '';
  return `${from.toLowerCase()}-${pluralize(to.toLowerCase())}`;
}

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
  | { kind: 'relationship'; index: number }
  | null;

/** Which editor surface is showing in the main region. */
export type EditorTab = 'form' | 'diagram';

/** What the diagram side panel is currently reflecting. */
export type DiagramSelection =
  | { kind: 'node'; id: string }
  | { kind: 'edge'; id: string }
  | null;

/**
 * The uncommitted draw-to-connect proposal. A gesture pre-fills from/to (and a
 * convenience name); nothing enters the draft until the proposal is committed.
 * Cancelling discards it and leaves the draft untouched.
 */
export interface ConnectProposal {
  from: string;
  to: string;
  name: string;
  cardinality: string;
  nameManual: boolean;
}

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
  /**
   * Why the session ended, so the terminal screen can say something true.
   * 'done' means the user ended it deliberately and the server shut down
   * because we asked; 'unreachable' means it went away underneath us. These
   * used to share one message asserting the server was unreachable, which is
   * false on every Done path.
   */
  sessionEndReason: 'done' | 'unreachable' | null;
  hadActivity: boolean;

  // validation surfaces (out-of-process, recomputed per response)
  findings: Finding[];
  validationPanelOpen: boolean;
  /** Element path currently highlighted from a finding, or null (whole-model / none). */
  selectedElement: string | null;
  /** How many validate calls have been issued — observed by the debounce test. */
  validateCallCount: number;
  /** Monotonic draft version; a validate response for an older version is discarded. */
  draftSeq: number;

  // relationships & diagram view state
  relNameManual: boolean;
  activeTab: EditorTab;
  diagramSelection: DiagramSelection;
  connectProposal: ConnectProposal | null;
  nodePositions: Record<string, { x: number; y: number }>;

  // lifecycle
  resetStore: () => void;
  hydrate: (env: ModelEnvelope) => void;
  load: () => Promise<void>;

  // selection
  selectEntity: (name: string) => void;
  selectEnum: (name: string) => void;

  // relationship editing (list + form)
  createRelationship: () => number;
  selectRelationship: (index: number) => void;
  openRelationship: (index: number) => void;
  deleteRelationship: (index: number) => void;
  setRelationshipEndpoint: (
    index: number,
    field: 'from' | 'to',
    value: string,
  ) => void;
  setRelationshipCardinality: (index: number, value: string) => void;
  setRelationshipName: (index: number, value: string) => void;

  // diagram view state
  setActiveTab: (tab: EditorTab) => void;
  selectNode: (id: string) => void;
  selectEdge: (id: string) => void;
  repositionNode: (id: string, pos: { x: number; y: number }) => void;

  // draw-to-connect proposal (uncommitted until committed)
  proposeRelationship: (from: string, to: string) => void;
  setProposalEndpoint: (field: 'from' | 'to', value: string) => void;
  setProposalCardinality: (value: string) => void;
  setProposalName: (value: string) => void;
  cancelProposal: () => void;
  commitProposal: () => number | null;

  // model mutation
  applyModel: (next: DomainModelDocument) => void;

  // persistence
  save: () => Promise<ModelEnvelope | null>;
  reloadAndReapply: () => Promise<void>;
  keepDraft: () => void;
  dismissConflict: () => void;
  clearServerError: () => void;

  // validation lifecycle
  /** Debounced trigger: a burst of edits collapses to one trailing validate. */
  scheduleRevalidation: () => void;
  /** Issue one validate call against the current draft; stale responses are discarded. */
  runValidation: () => Promise<void>;
  openValidationPanel: () => void;
  closeValidationPanel: () => void;
  /** Navigate to a finding's owning element and highlight it; whole-model highlights nothing. */
  selectFinding: (finding: Finding) => void;
}

// Debounce window for trailing revalidation. Module-scoped so a rapid burst of
// edits across store actions collapses into a single trailing validate call.
const REVALIDATE_DEBOUNCE_MS = 250;
let revalidateTimer: ReturnType<typeof setTimeout> | null = null;

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
  sessionEndReason: null,
  hadActivity: false,
  findings: [],
  validationPanelOpen: false,
  selectedElement: null,
  validateCallCount: 0,
  draftSeq: 0,
  relNameManual: false,
  activeTab: 'form',
  diagramSelection: null,
  connectProposal: null,
  nodePositions: {},

  resetStore: () => {
    if (revalidateTimer) {
      clearTimeout(revalidateTimer);
      revalidateTimer = null;
    }
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
      sessionEndReason: null,
      hadActivity: false,
      findings: [],
      validationPanelOpen: false,
      selectedElement: null,
      validateCallCount: 0,
      draftSeq: 0,
      relNameManual: false,
      activeTab: 'form',
      diagramSelection: null,
      connectProposal: null,
      nodePositions: {},
    });
  },

  hydrate: (env) =>
    set((s) => ({
      model: env.model,
      etag: env.etag,
      isDirty: false,
      fieldErrors: [],
      conflict: null,
      serverError: null,
      // Findings are recomputed from scratch on the load-time revalidation; a
      // fresh model discards the prior finding set and bumps the draft version
      // so any in-flight validate response is dropped.
      findings: [],
      selectedElement: null,
      draftSeq: s.draftSeq + 1,
      // Diagram view state is derived, not persisted: a reload re-lays-out
      // deterministically and discards any in-session node repositioning.
      diagramSelection: null,
      connectProposal: null,
      nodePositions: {},
    })),

  load: async () => {
    set({ isLoading: true });
    try {
      const env = await loadModel();
      get().hydrate(env);
      // Load-time revalidation: a freshly-loaded invalid file shows findings —
      // and the blocked save bar — before any edit.
      await get().runValidation();
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

  // --- relationship editing -------------------------------------------------

  createRelationship: () => {
    const { model } = get();
    const relationships = model.relationships ?? [];
    // A blank draft relationship: endpoints and cardinality are chosen in the
    // form; the name pre-fills once both endpoints are set.
    const rel: DomainRelationship = {
      name: '',
      from: '',
      to: '',
      cardinality: '',
    };
    const index = relationships.length;
    set({
      model: { ...model, relationships: [...relationships, rel] },
      isDirty: true,
      hadActivity: true,
      fieldErrors: [],
      selection: { kind: 'relationship', index },
      relNameManual: false,
    });
    return index;
  },

  selectRelationship: (index) =>
    set({ selection: { kind: 'relationship', index }, relNameManual: false }),

  openRelationship: (index) =>
    set({ selection: { kind: 'relationship', index }, relNameManual: false }),

  deleteRelationship: (index) => {
    // Delete is immediate — nothing in the model references a relationship by
    // name, so there are no dependents to check.
    const { model, selection } = get();
    const relationships = model.relationships ?? [];
    const next = relationships.filter((_, i) => i !== index);
    const clearSel =
      selection?.kind === 'relationship' && selection.index === index;
    set({
      model: { ...model, relationships: next },
      isDirty: true,
      hadActivity: true,
      selection: clearSel ? null : selection,
    });
  },

  setRelationshipEndpoint: (index, field, value) => {
    const { model, relNameManual } = get();
    const relationships = [...(model.relationships ?? [])];
    const current = relationships[index];
    if (!current) return;
    const nextRel: DomainRelationship = { ...current, [field]: value };
    // Regenerate the convenience name from the endpoints until it is manually
    // edited; after a manual rename, endpoint changes leave the name alone.
    if (!relNameManual) {
      const prefill = relationshipNamePrefill(nextRel.from, nextRel.to);
      if (prefill) nextRel.name = prefill;
    }
    relationships[index] = nextRel;
    set({
      model: { ...model, relationships },
      isDirty: true,
      hadActivity: true,
    });
  },

  setRelationshipCardinality: (index, value) => {
    const { model } = get();
    const relationships = [...(model.relationships ?? [])];
    const current = relationships[index];
    if (!current) return;
    relationships[index] = { ...current, cardinality: value };
    set({
      model: { ...model, relationships },
      isDirty: true,
      hadActivity: true,
    });
  },

  setRelationshipName: (index, value) => {
    const { model } = get();
    const relationships = [...(model.relationships ?? [])];
    const current = relationships[index];
    if (!current) return;
    relationships[index] = { ...current, name: value };
    // A manual edit stops the endpoint-driven prefill from regenerating it.
    set({
      model: { ...model, relationships },
      isDirty: true,
      hadActivity: true,
      relNameManual: true,
    });
  },

  // --- diagram view state ---------------------------------------------------

  setActiveTab: (tab) => set({ activeTab: tab }),

  selectNode: (id) =>
    set({
      diagramSelection: { kind: 'node', id },
      selection: { kind: 'entity', name: id },
    }),

  selectEdge: (id) => {
    const { model } = get();
    const relationships = model.relationships ?? [];
    const index = relationships.findIndex((r) => r.name === id);
    set({
      diagramSelection: { kind: 'edge', id },
      selection:
        index >= 0 ? { kind: 'relationship', index } : get().selection,
      relNameManual: index >= 0,
    });
  },

  // Manual repositioning is view-only: it never marks the draft dirty and is
  // discarded on reload (deterministic re-layout).
  repositionNode: (id, pos) =>
    set((s) => ({ nodePositions: { ...s.nodePositions, [id]: pos } })),

  // --- draw-to-connect proposal (uncommitted) -------------------------------

  proposeRelationship: (from, to) =>
    set({
      connectProposal: {
        from,
        to,
        name: relationshipNamePrefill(from, to),
        // Seed a sensible default so a proposal always carries a cardinality;
        // the form lets the designer change it before committing.
        cardinality: 'one-to-many',
        nameManual: false,
      },
    }),

  setProposalEndpoint: (field, value) => {
    const p = get().connectProposal;
    if (!p) return;
    const next: ConnectProposal = { ...p, [field]: value };
    if (!p.nameManual) {
      const prefill = relationshipNamePrefill(next.from, next.to);
      if (prefill) next.name = prefill;
    }
    set({ connectProposal: next });
  },

  setProposalCardinality: (value) => {
    const p = get().connectProposal;
    if (!p) return;
    set({ connectProposal: { ...p, cardinality: value } });
  },

  setProposalName: (value) => {
    const p = get().connectProposal;
    if (!p) return;
    set({ connectProposal: { ...p, name: value, nameManual: true } });
  },

  cancelProposal: () => set({ connectProposal: null }),

  commitProposal: () => {
    const { model, connectProposal } = get();
    if (!connectProposal) return null;
    const relationships = model.relationships ?? [];
    const name = connectProposal.name.trim();
    // Commit is only legal with a unique name and a chosen cardinality.
    const duplicate = relationships.some((r) => r.name === name);
    if (name === '' || connectProposal.cardinality === '' || duplicate) {
      return null;
    }
    const rel: DomainRelationship = {
      name,
      from: connectProposal.from,
      to: connectProposal.to,
      cardinality: connectProposal.cardinality,
    };
    const index = relationships.length;
    set({
      model: { ...model, relationships: [...relationships, rel] },
      isDirty: true,
      hadActivity: true,
      connectProposal: null,
      selection: { kind: 'relationship', index },
      relNameManual: true,
    });
    return index;
  },

  applyModel: (next) => {
    set((s) => ({
      model: next,
      isDirty: true,
      hadActivity: true,
      fieldErrors: [],
      draftSeq: s.draftSeq + 1,
    }));
    // A committed mutation triggers the debounced trailing revalidation.
    get().scheduleRevalidation();
  },

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
            if (get().hadActivity) {
              set({ sessionEnded: true, sessionEndReason: 'unreachable' });
            }
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
      diagramSelection: null,
      connectProposal: null,
      nodePositions: {},
    });
  },

  keepDraft: () => set({ conflict: null }),
  dismissConflict: () => set({ conflict: null }),
  clearServerError: () => set({ serverError: null }),

  // --- validation lifecycle -------------------------------------------------

  scheduleRevalidation: () => {
    // Collapse a burst of edits into one trailing call against the final draft.
    if (revalidateTimer) clearTimeout(revalidateTimer);
    revalidateTimer = setTimeout(() => {
      revalidateTimer = null;
      void get().runValidation();
    }, REVALIDATE_DEBOUNCE_MS);
  },

  runValidation: async () => {
    // Tag this call to the draft version it was issued for; a response that a
    // newer draft has superseded is discarded, never rendered over fresher
    // findings.
    const issuedFor = get().draftSeq;
    set((s) => ({ validateCallCount: s.validateCallCount + 1 }));
    try {
      const findings = await validateModel(get().model);
      if (get().draftSeq !== issuedFor) return; // stale — discard
      // Findings recomputed from scratch per response (no client-side cache).
      set({ findings });
    } catch {
      // A malformed request / transport failure leaves the prior findings in
      // place rather than clearing them; validation is presentation-only and
      // never blocks editing. Errors are swallowed here by design.
    }
  },

  openValidationPanel: () => set({ validationPanelOpen: true }),
  closeValidationPanel: () => set({ validationPanelOpen: false }),

  selectFinding: (finding) => {
    // A whole-model finding (distinguished top-level path) anchors to nothing:
    // highlight nothing and leave the current selection untouched.
    if (finding.field === WHOLE_MODEL_PATH || finding.field === '') {
      set({ selectedElement: null });
      return;
    }
    const target = resolveElementSelection(get().model, finding.field);
    set({
      selectedElement: finding.field,
      selection: target ?? get().selection,
      activeTab: 'form',
    });
  },
}));

// Element-path navigation --------------------------------------------------

/**
 * Resolve a finding's dotted element path to its owning editor surface:
 *   entities.<E>.fields.<F>[.type]  → the entity form
 *   enums.<E>.values.<V>            → the enum form
 *   relationships.<R>.<end>         → the relationship form
 * Returns null when the path names the whole model or cannot be resolved.
 */
export function resolveElementSelection(
  model: DomainModelDocument,
  path: string,
): Selection {
  const parts = path.split('.');
  if (parts[0] === 'entities' && parts[1]) {
    return { kind: 'entity', name: parts[1] };
  }
  if (parts[0] === 'enums' && parts[1]) {
    return { kind: 'enum', name: parts[1] };
  }
  if (parts[0] === 'relationships' && parts[1]) {
    const index = (model.relationships ?? []).findIndex(
      (r) => r.name === parts[1],
    );
    if (index >= 0) return { kind: 'relationship', index };
  }
  return null;
}

// Convenience selectors ------------------------------------------------------

export function selectedEntity(state: EditorState): DomainEntity | null {
  if (state.selection?.kind !== 'entity') return null;
  const { name } = state.selection;
  return state.model.entities.find((e) => e.name === name) ?? null;
}

export function selectedEnum(state: EditorState): DomainEnum | null {
  if (state.selection?.kind !== 'enum') return null;
  const { name } = state.selection;
  return state.model.enums.find((e) => e.name === name) ?? null;
}

export function selectedRelationship(
  state: EditorState,
): DomainRelationship | null {
  if (state.selection?.kind !== 'relationship') return null;
  return state.model.relationships?.[state.selection.index] ?? null;
}

/** Index of the currently-selected relationship, or -1. */
export function selectedRelationshipIndex(state: EditorState): number {
  return state.selection?.kind === 'relationship' ? state.selection.index : -1;
}

// Validation selectors -------------------------------------------------------

export function errorFindings(state: EditorState): Finding[] {
  return state.findings.filter((f) => f.severity === 'error');
}

export function warningFindings(state: EditorState): Finding[] {
  return state.findings.filter((f) => f.severity === 'warning');
}

export function errorCount(state: EditorState): number {
  return errorFindings(state).length;
}

export function warningCount(state: EditorState): number {
  return warningFindings(state).length;
}

/** A model is clean exactly when the current validate response had no findings. */
export function isClean(state: EditorState): boolean {
  return state.findings.length === 0;
}

/** The save is blocked exactly while any error-severity finding exists. */
export function isBlocked(state: EditorState): boolean {
  return errorCount(state) > 0;
}

/**
 * Map from element path to the finding anchored there, for the inline markers
 * and navigation. Whole-model findings are excluded — they anchor to nothing.
 */
export function findingsByPath(state: EditorState): Record<string, Finding> {
  const out: Record<string, Finding> = {};
  for (const f of state.findings) {
    if (f.field && f.field !== WHOLE_MODEL_PATH) out[f.field] = f;
  }
  return out;
}

/**
 * The finding anchored at `path`, or null. Anchoring tolerates Core's optional
 * trailing sub-path on a finding (e.g. entities.E.fields.F.type,
 * enums.E.values.V.tone), matching it to the row at entities.E.fields.F /
 * enums.E.values.V. An exact match or a `path.`-prefixed match anchors; a
 * sibling with a longer name (statusX vs status) does not.
 */
export function findingForElement(
  state: EditorState,
  path: string,
): Finding | null {
  for (const f of state.findings) {
    if (!f.field || f.field === WHOLE_MODEL_PATH) continue;
    if (f.field === path || f.field.startsWith(`${path}.`)) return f;
  }
  return null;
}
