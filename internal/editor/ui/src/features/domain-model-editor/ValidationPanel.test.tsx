// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: validation-panel
// parlay-artifact: test
import { render, screen, fireEvent, act } from '@testing-library/react';
import { ValidationPanel } from './ValidationPanel';
import { useEditorStore } from '../../store/editorStore';
import { clone, populatedEnvelope } from '../../test/fixtures';
import type { Finding } from '../../lib/api';

const orphanFinding: Finding = {
  field: 'entities.Order.fields.status',
  code: 'undeclared-entity-reference',
  severity: 'error',
  message: 'Order.status names the deleted enum OrderStatus',
  fix: 'declare OrderStatus or retype the field',
};

const deprecatedWarning: Finding = {
  field: 'operations',
  code: 'domain-operations-deprecated',
  severity: 'warning',
  message: 'top-level operations are deprecated',
};

const wholeModelFinding: Finding = {
  field: '<domain-model>',
  code: 'missing-schema-version',
  severity: 'error',
  message: 'the model declares no schema_version',
  fix: 'add schema_version: 1',
};

function setFindings(findings: Finding[]) {
  act(() => useEditorStore.setState({ findings }));
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
  useEditorStore.getState().hydrate(clone(populatedEnvelope));
});

describe('ValidationPanel', () => {
  it('renders a load-time orphan reference as one error-styled row with its element path', () => {
    setFindings([orphanFinding]);
    render(<ValidationPanel />);

    expect(screen.getAllByTestId('finding-rows')).toHaveLength(1);
    expect(screen.getByTestId('element-path')).toHaveTextContent(
      'entities.Order.fields.status',
    );
    const badge = screen.getByTestId('severity-badge');
    expect(badge).toBeVisible();
    expect(screen.getByTestId('finding-rows')).toHaveAttribute(
      'data-severity',
      'error',
    );
  });

  it('shows the schema fix message alongside the code, not just the bare code', () => {
    setFindings([orphanFinding]);
    render(<ValidationPanel />);
    expect(screen.getByTestId('finding-message')).toBeVisible();
    expect(screen.getByTestId('finding-message')).toHaveTextContent(
      'declare OrderStatus',
    );
    expect(screen.getByTestId('finding-code')).toHaveTextContent(
      'undeclared-entity-reference',
    );
  });

  it('clicking a finding navigates to and highlights its owning element', () => {
    // A relationship exists so a relationships.* path resolves to it.
    act(() =>
      useEditorStore.setState({
        model: {
          schema_version: 1,
          enums: [],
          entities: [],
          relationships: [
            { name: 'customer-orders', from: 'Customer', to: 'Order', cardinality: 'one-to-many' },
          ],
        },
        findings: [
          {
            field: 'relationships.customer-orders.to',
            code: 'relationship-cardinality-unknown',
            severity: 'error',
            message: 'unknown cardinality',
          },
        ],
      }),
    );
    render(<ValidationPanel />);

    fireEvent.click(screen.getByTestId('navigate-to-element'));

    const state = useEditorStore.getState();
    expect(state.selectedElement).toBe('relationships.customer-orders.to');
    expect(state.selection).toEqual({ kind: 'relationship', index: 0 });
  });

  it('a whole-model finding highlights nothing but keeps its fix text in place', () => {
    setFindings([wholeModelFinding]);
    render(<ValidationPanel />);

    fireEvent.click(screen.getByTestId('navigate-to-element'));

    expect(useEditorStore.getState().selectedElement).toBeNull();
    // The row and its fix text remain in place.
    expect(screen.getByTestId('finding-message')).toBeVisible();
    // A whole-model finding anchors to nothing — no element path is rendered.
    expect(screen.queryByTestId('element-path')).not.toBeInTheDocument();
  });

  it('renders warnings visually distinct from errors', () => {
    setFindings([orphanFinding, deprecatedWarning]);
    render(<ValidationPanel />);

    const rows = screen.getAllByTestId('finding-rows');
    expect(rows).toHaveLength(2);
    const severities = rows.map((r) => r.getAttribute('data-severity')).sort();
    expect(severities).toEqual(['error', 'warning']);
  });

  it('renders the explicit clean state for zero findings, not merely no rows', () => {
    setFindings([]);
    render(<ValidationPanel />);
    expect(screen.getByTestId('empty-clean-state')).toBeVisible();
    expect(screen.queryByTestId('finding-rows')).not.toBeInTheDocument();
  });
});
