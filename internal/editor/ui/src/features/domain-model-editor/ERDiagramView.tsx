// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: er-diagram-view
import { useMemo } from 'react';
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  Handle,
  Position,
  type Node,
  type Edge,
  type Connection,
  type NodeProps,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { useEditorStore } from '../../store/editorStore';
import { computeLayout } from './erLayout';
import type { DomainEntity } from '../../types/domain';
import { EntityFormPanel } from './EntityFormPanel';
import { RelationshipFormPanel } from './RelationshipFormPanel';

// React Flow custom node: one card per entity, with its field list and the
// source/target handles that make the draw-to-connect gesture possible.
// Defined module-level so its identity is stable across renders.
function EntityNode({ data }: NodeProps) {
  const entity = data.entity as DomainEntity;
  return (
    <div
      data-testid="er-flow-node"
      className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm shadow-sm"
    >
      <Handle type="target" position={Position.Left} />
      <div className="font-semibold text-slate-800">{entity.name}</div>
      <ul className="mt-1 flex flex-col gap-0.5">
        {entity.fields.map((f) => (
          <li key={f.name} className="text-xs text-slate-500">
            {f.name}: {f.type}
            {f.target ? ` → ${f.target}` : ''}
          </li>
        ))}
      </ul>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

const nodeTypes = { entity: EntityNode };

/**
 * A React-Flow graph of the whole model (the adapter's domain-model-er-diagram
 * composition). One node per entity (name + field list), one edge per
 * relationship (labelled name + cardinality). A `ref`-typed field renders
 * inside its node's field list, never as an edge — the diagram distinguishes
 * the schema's two reference constructs.
 *
 * The diagram and the form panels are two views over one in-memory draft:
 * clicking a node opens the entity form in the side panel, clicking an edge
 * opens the relationship form, and an edit in either is immediately reflected
 * in the other. Draw-to-connect (dragging a handle from one node to another, or
 * back to the same node for a self-loop) opens the relationship form pre-filled
 * with from/to as an uncommitted proposal; committing creates the relationship,
 * cancelling creates nothing.
 *
 * Auto-layout is deterministic (see erLayout). Manual node dragging repositions
 * within the session only and never marks the draft dirty. Pan and zoom are
 * always available — no minimum-entity-count gate.
 *
 * An accessible graph summary mirrors the computed layout so the diagram's
 * structure is reachable without the canvas.
 */
export function ERDiagramView() {
  const model = useEditorStore((s) => s.model);
  const nodePositions = useEditorStore((s) => s.nodePositions);
  const diagramSelection = useEditorStore((s) => s.diagramSelection);
  const connectProposal = useEditorStore((s) => s.connectProposal);
  const selectNode = useEditorStore((s) => s.selectNode);
  const selectEdge = useEditorStore((s) => s.selectEdge);
  const repositionNode = useEditorStore((s) => s.repositionNode);
  const proposeRelationship = useEditorStore((s) => s.proposeRelationship);

  const layout = useMemo(() => computeLayout(model), [model]);

  const rfNodes: Node[] = layout.nodes.map((n) => ({
    id: n.id,
    type: 'entity',
    position: nodePositions[n.id] ?? n.position,
    data: { entity: n.entity },
  }));

  const rfEdges: Edge[] = layout.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    label: `${e.relationship.name} · ${e.relationship.cardinality}`,
  }));

  const onConnect = (c: Connection) => {
    if (c.source && c.target) proposeRelationship(c.source, c.target);
  };

  return (
    <div className="flex h-full min-h-[24rem] flex-col">
      <div className="relative min-h-[24rem] flex-1" data-testid="er-canvas">
        <ReactFlowProvider>
          <ReactFlow
            nodes={rfNodes}
            edges={rfEdges}
            nodeTypes={nodeTypes}
            onConnect={onConnect}
            onNodeClick={(_, node) => selectNode(node.id)}
            onEdgeClick={(_, edge) => selectEdge(edge.id)}
            onNodeDragStop={(_, node) => repositionNode(node.id, node.position)}
            fitView
          >
            <Background />
            <Controls />
          </ReactFlow>
        </ReactFlowProvider>
      </div>

      {/* Accessible, deterministic summary of the graph (mirrors computeLayout). */}
      <div
        data-testid="er-graph-summary"
        aria-label="Diagram summary"
        className="border-t border-slate-200 p-3"
      >
        <ul className="flex flex-col gap-2">
          {layout.nodes.map((n) => (
            <li
              key={n.id}
              data-testid="entity-node"
              data-entity={n.id}
              className="rounded border border-slate-200 p-2"
            >
              <button
                type="button"
                data-testid="click-node"
                data-node={n.id}
                onClick={() => selectNode(n.id)}
                className="text-sm font-medium text-slate-800"
              >
                {n.entity.name}
              </button>
              <div className="mt-1 flex flex-wrap gap-1">
                {n.entity.fields.map((f) => (
                  <span
                    key={f.name}
                    data-testid="field-badge"
                    data-field={f.name}
                    className="rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 text-xs text-slate-600"
                  >
                    {f.name}: {f.type}
                    {f.target ? ` → ${f.target}` : ''}
                  </span>
                ))}
              </div>
            </li>
          ))}
        </ul>

        <ul className="mt-2 flex flex-col gap-1">
          {layout.edges.map((e) => (
            <li
              key={e.id}
              data-testid="relationship-edge"
              data-edge={e.id}
              className="flex items-center gap-2 text-sm text-slate-700"
            >
              <button
                type="button"
                data-testid="click-edge"
                data-edge={e.id}
                onClick={() => selectEdge(e.id)}
                className="font-medium text-slate-800"
              >
                {e.relationship.name}
              </button>
              <span className="text-xs text-slate-400">
                {e.source} → {e.target}
              </span>
              <span
                data-testid="cardinality-markers"
                data-cardinality={e.relationship.cardinality}
                className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-600"
              >
                {e.relationship.cardinality}
              </span>
            </li>
          ))}
        </ul>
      </div>

      {/* Node side panel: the entity form, over the same draft. */}
      {diagramSelection?.kind === 'node' && (
        <aside
          data-testid="node-side-panel"
          className="border-t border-slate-200 bg-white"
        >
          <EntityFormPanel />
        </aside>
      )}

      {/* Edge side panel: the relationship form, over the same draft. */}
      {diagramSelection?.kind === 'edge' && (
        <aside
          data-testid="edge-side-panel"
          className="border-t border-slate-200 bg-white"
        >
          <RelationshipFormPanel />
        </aside>
      )}

      {/* Draw-to-connect: the pre-filled, uncommitted proposal form. */}
      {connectProposal && (
        <aside
          data-testid="connect-proposal-form"
          className="border-t border-slate-200 bg-white"
        >
          <RelationshipFormPanel proposal />
        </aside>
      )}
    </div>
  );
}

export default ERDiagramView;
