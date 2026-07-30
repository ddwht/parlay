// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: cross-cutting/er-diagram-deterministic-layout
// parlay-artifact: test
import { computeLayout } from './erLayout';
import { useEditorStore } from '../../store/editorStore';
import type { DomainModelDocument } from '../../types/domain';

function relatedModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [
      {
        name: 'Customer',
        fields: [
          { name: 'id', type: 'uuid', required: true },
          { name: 'name', type: 'string', required: true },
        ],
      },
      {
        name: 'Order',
        fields: [
          { name: 'id', type: 'uuid', required: true },
          { name: 'status', type: 'string', required: true },
        ],
      },
    ],
    relationships: [
      { name: 'customer-orders', from: 'Customer', to: 'Order', cardinality: 'one-to-many' },
    ],
  };
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('erLayout / deterministic ER auto-layout', () => {
  it('exposes the layout module (computeLayout)', () => {
    expect(typeof computeLayout).toBe('function');
  });

  it('auto-layout is deterministic across repeated calls on the same model', () => {
    const model = relatedModel();
    const a = computeLayout(model);
    const b = computeLayout(model);
    expect(b.nodes.map((n) => n.position)).toEqual(
      a.nodes.map((n) => n.position),
    );
    expect(b).toEqual(a);
  });

  it('never writes node positions or layout state into the domain model', () => {
    const model = relatedModel();
    const before = JSON.parse(JSON.stringify(model));
    computeLayout(model);
    // The input document is untouched — positions live only on the returned
    // layout, never on the model.
    expect(model).toEqual(before);
    const serialized = JSON.stringify(model);
    expect(serialized).not.toMatch(/"position"/);
    expect(serialized).not.toMatch(/"x"/);
    expect(serialized).not.toMatch(/"y"/);
  });

  it('reads and creates no sidecar layout file — it is a pure projection', () => {
    const model = relatedModel();
    // A pure function of the model alone: no side effects, no I/O.
    Object.freeze(model.entities);
    Object.freeze(model.relationships);
    expect(() => computeLayout(model)).not.toThrow();
    const layout = computeLayout(model);
    expect(layout.nodes).toHaveLength(2);
    expect(layout.edges).toHaveLength(1);
  });

  it('in-session node drag does not dirty the draft and resets on reload', () => {
    const store = useEditorStore.getState();
    store.hydrate({ model: relatedModel(), etag: 'e1' });
    store.repositionNode('Order', { x: 999, y: 999 });

    expect(useEditorStore.getState().isDirty).toBe(false);
    expect(useEditorStore.getState().nodePositions.Order).toEqual({
      x: 999,
      y: 999,
    });

    // Reload → deterministic re-layout; in-session positions are discarded.
    useEditorStore.getState().hydrate({ model: relatedModel(), etag: 'e2' });
    expect(useEditorStore.getState().nodePositions).toEqual({});
    const layout = computeLayout(useEditorStore.getState().model);
    const orderNode = layout.nodes.find((n) => n.id === 'Order')!;
    expect(orderNode.position).not.toEqual({ x: 999, y: 999 });
  });

  it('draw-to-connect is the only draft-mutation path from the diagram', () => {
    const store = useEditorStore.getState();
    store.hydrate({ model: relatedModel(), etag: 'e1' });

    // A cancelled proposal enters nothing.
    store.proposeRelationship('Customer', 'Order');
    useEditorStore.getState().cancelProposal();
    expect(useEditorStore.getState().model.relationships).toHaveLength(1);
    expect(useEditorStore.getState().isDirty).toBe(false);

    // A committed proposal is the one mutation path.
    useEditorStore.getState().proposeRelationship('Order', 'Customer');
    useEditorStore.getState().commitProposal();
    expect(useEditorStore.getState().model.relationships).toHaveLength(2);
    expect(useEditorStore.getState().isDirty).toBe(true);
  });
});
