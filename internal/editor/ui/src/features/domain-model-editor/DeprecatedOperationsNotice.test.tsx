// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: deprecated-operations-notice
// parlay-artifact: test
import { render, screen } from '@testing-library/react';
import { DeprecatedOperationsNotice } from './DeprecatedOperationsNotice';
import { useEditorStore } from '../../store/editorStore';
import type { DomainModelDocument } from '../../types/domain';

function operationsModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [{ name: 'Customer', fields: [] }],
    relationships: [],
    operations: [
      { name: 'placeOrder', kind: 'command' },
      { name: 'listOrders', kind: 'query' },
    ],
  };
}

function cleanModel(): DomainModelDocument {
  return {
    schema_version: 1,
    enums: [],
    entities: [
      { name: 'Customer', fields: [] },
      { name: 'Order', fields: [] },
    ],
    relationships: [
      { name: 'customer-orders', from: 'Customer', to: 'Order', cardinality: 'one-to-many' },
    ],
  };
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('DeprecatedOperationsNotice', () => {
  it('renders read-only entries and the migration command when operations are present', () => {
    useEditorStore.getState().hydrate({ model: operationsModel(), etag: 'e1' });
    render(<DeprecatedOperationsNotice />);

    expect(screen.getByTestId('notice-panel')).toBeInTheDocument();
    expect(screen.getAllByTestId('operation-entries')).toHaveLength(2);
    expect(screen.getByTestId('migration-pointer')).toHaveTextContent(
      'parlay migrate-domain-operations',
    );
  });

  it('renders nothing at all on a model with no operations field', () => {
    useEditorStore.getState().hydrate({ model: cleanModel(), etag: 'e1' });
    render(<DeprecatedOperationsNotice />);
    expect(screen.queryByTestId('notice-panel')).not.toBeInTheDocument();
    expect(screen.queryByTestId('operation-entries')).not.toBeInTheDocument();
    expect(screen.queryByTestId('migration-pointer')).not.toBeInTheDocument();
  });

  it('is informational only — entries are visible and the sole action is acknowledge', () => {
    useEditorStore.getState().hydrate({ model: operationsModel(), etag: 'e1' });
    render(<DeprecatedOperationsNotice />);
    // The entries are shown (not hidden) and there is no edit/delete affordance.
    expect(screen.getAllByTestId('operation-entries')[0]).toBeVisible();
    expect(screen.getByTestId('acknowledge-notice')).toBeEnabled();
  });

  it('leaves the operations block byte-for-byte identical across an unrelated edit', () => {
    useEditorStore.getState().hydrate({ model: operationsModel(), etag: 'e1' });
    const before = JSON.stringify(
      useEditorStore.getState().model.operations,
    );

    // An unrelated entity edit must not touch the deprecated operations block.
    const model = useEditorStore.getState().model;
    useEditorStore.getState().applyModel({
      ...model,
      entities: [...model.entities, { name: 'Product', fields: [] }],
    });

    const after = JSON.stringify(useEditorStore.getState().model.operations);
    expect(after).toBe(before);
  });
});
