// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: inline-finding-marker
// parlay-artifact: test
import { render, screen, fireEvent, act } from '@testing-library/react';
import { InlineFindingMarker } from './InlineFindingMarker';
import { useEditorStore } from '../../store/editorStore';
import type { Finding } from '../../lib/api';

const PATH = 'entities.Order.fields.status';

const orphanError: Finding = {
  field: PATH,
  code: 'undeclared-entity-reference',
  severity: 'error',
  message: 'Order.status names a deleted enum',
  fix: 'declare the enum or retype the field',
};

const warning: Finding = {
  field: PATH,
  code: 'domain-operations-deprecated',
  severity: 'warning',
  message: 'deprecated',
};

function setFindings(findings: Finding[]) {
  act(() => useEditorStore.setState({ findings }));
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

describe('InlineFindingMarker', () => {
  it('renders an inline error marker at the field row for an anchored finding', () => {
    setFindings([orphanError]);
    render(<InlineFindingMarker path={PATH} />);
    const badge = screen.getByTestId('marker-badge');
    expect(badge).toBeVisible();
    expect(badge).toHaveAttribute('data-severity', 'error');
  });

  it('carries the finding message on inspect', () => {
    setFindings([orphanError]);
    render(<InlineFindingMarker path={PATH} />);
    expect(screen.queryByTestId('marker-message')).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId('marker-badge'));
    expect(screen.getByTestId('marker-message')).toBeVisible();
  });

  it('renders a warning marker visually distinct from an error marker', () => {
    setFindings([warning]);
    render(<InlineFindingMarker path={PATH} />);
    expect(screen.getByTestId('marker-badge')).toHaveAttribute(
      'data-severity',
      'warning',
    );
  });

  it('clears the inline marker once the element has no finding', () => {
    // No finding for this element (a clean draft) — the marker does not render.
    setFindings([]);
    render(<InlineFindingMarker path={PATH} />);
    expect(screen.queryByTestId('marker-badge')).not.toBeInTheDocument();
  });
});
