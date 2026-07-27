// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: save-gating-state
// parlay-artifact: test
import { render, screen, fireEvent, act } from '@testing-library/react';
import { SaveGatingState } from './SaveGatingState';
import { useEditorStore } from '../store/editorStore';
import type { Finding } from '../lib/api';

const err = (field: string): Finding => ({
  field,
  code: 'field-type-outside-closed-set',
  severity: 'error',
  message: 'bad type',
});

const warningOnly: Finding = {
  field: 'operations',
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

describe('SaveGatingState', () => {
  it('shows the blocked state naming the count on load, before any edit', () => {
    // Two load-time errors, present before any edit.
    setFindings([err('entities.Order.fields.a'), err('entities.Order.fields.b')]);
    render(<SaveGatingState />);
    expect(screen.getByTestId('blocked-save-bar')).toBeVisible();
    expect(screen.getByTestId('blocked-save-bar')).toHaveTextContent('2 errors');
  });

  it('links to the validation panel rather than a bare disabled button', () => {
    setFindings([err('entities.Order.fields.a')]);
    render(<SaveGatingState />);
    fireEvent.click(screen.getByTestId('view-problems'));
    expect(useEditorStore.getState().validationPanelOpen).toBe(true);
  });

  it('does not gate on warning-only findings — the normal save affordance shows', () => {
    setFindings([warningOnly]);
    render(<SaveGatingState />);
    expect(screen.queryByTestId('blocked-save-bar')).not.toBeInTheDocument();
  });

  it('transitions back to the normal state once the errors are fixed', () => {
    setFindings([]);
    render(<SaveGatingState />);
    expect(screen.queryByTestId('blocked-save-bar')).not.toBeInTheDocument();
  });
});
