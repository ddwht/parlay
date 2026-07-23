// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-section: cross-cutting
import type {
  DomainModelDocument,
  DomainEntity,
  DomainRelationship,
} from '../../types/domain';

/**
 * Deterministic ER auto-layout for the domain-model diagram.
 *
 * `computeLayout(model)` is a PURE read-projection of the model: the same model
 * always yields the same node arrangement, so a reload after save reproduces
 * the prior arrangement rather than surprising the designer. It holds no
 * persisted state, reads no sidecar file, and NEVER writes node positions or
 * any layout key back into the domain model — the serialized document stays
 * pure domain vocabulary.
 *
 * Nodes are one-per-entity. Edges are one-per-relationship; a `ref`-typed field
 * is a field on its entity's node, NOT an edge, so it never appears here.
 * Manual in-session node repositioning is view-only React-Flow state (held in
 * the editor store), never fed back into this function and never persisted.
 */

export interface ErPosition {
  x: number;
  y: number;
}

/** A layout node — one per entity. `entity` is the source-of-truth vocabulary. */
export interface ErNode {
  id: string;
  position: ErPosition;
  entity: DomainEntity;
}

/** A layout edge — one per relationship. `ref` fields never produce an edge. */
export interface ErEdge {
  id: string;
  source: string;
  target: string;
  relationship: DomainRelationship;
}

export interface ErLayout {
  nodes: ErNode[];
  edges: ErEdge[];
}

// Grid geometry. Deterministic: position is a pure function of the entity's
// declaration index, so identical models always lay out identically.
const COLUMNS = 3;
const COLUMN_GAP = 280;
const ROW_GAP = 200;

/** Position an entity deterministically from its declaration index. */
export function nodePosition(index: number): ErPosition {
  const col = index % COLUMNS;
  const row = Math.floor(index / COLUMNS);
  return { x: col * COLUMN_GAP, y: row * ROW_GAP };
}

/**
 * Derive the diagram's nodes and edges from the model alone. Declaration order
 * is preserved for both entities and relationships.
 */
export function computeLayout(model: DomainModelDocument): ErLayout {
  const nodes: ErNode[] = model.entities.map((entity, index) => ({
    id: entity.name,
    position: nodePosition(index),
    entity,
  }));

  const relationships = model.relationships ?? [];
  const edges: ErEdge[] = relationships.map((relationship) => ({
    // React Flow requires a stable, unique edge id. Relationship names are
    // unique within a model, so the name is a natural id.
    id: relationship.name,
    source: relationship.from,
    target: relationship.to,
    relationship,
  }));

  return { nodes, edges };
}
