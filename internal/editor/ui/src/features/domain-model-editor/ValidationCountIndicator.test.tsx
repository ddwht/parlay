// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: validation-count-indicator
// parlay-artifact: test
import { render, screen, fireEvent, act } from '@testing-library/react';
import { ValidationCountIndicator } from './ValidationCountIndicator';
import { useEditorStore } from '../../store/editorStore';
import type { Finding } from '../../lib/api';

const errorFinding: Finding = {
  field: 'entities.Order.fields.qty',
  code: 'field-type-outside-closed-set',
  severity: 'error',
  message: 'bad type',
};
const warningFinding: Finding = {
  field: 'operations',
  code: 'domain-operations-deprecated',
  severity: 'warning',
  message: 'deprecated',
};

function emptyModel() {
  return { schema_version: 1, enums: [], entities: [] };
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('ValidationCountIndicator', () => {
  it('counts and styles errors and warnings distinctly for a mixed-severity model', () => {
    act(() =>
      useEditorStore.setState({ findings: [errorFinding, warningFinding] }),
    );
    render(<ValidationCountIndicator />);
    expect(screen.getByTestId('error-count-badge')).toBeVisible();
    expect(screen.getByTestId('warning-count-badge')).toBeVisible();
    expect(screen.getByTestId('error-count-badge')).toHaveTextContent('1 error');
    expect(screen.getByTestId('warning-count-badge')).toHaveTextContent(
      '1 warning',
    );
  });

  it('flips to the explicit clean state when the last finding clears', () => {
    act(() => useEditorStore.setState({ findings: [] }));
    render(<ValidationCountIndicator />);
    expect(screen.getByTestId('clean-state')).toBeVisible();
    expect(screen.queryByTestId('error-count-badge')).not.toBeInTheDocument();
  });

  it('opens the validation panel on click', () => {
    act(() => useEditorStore.setState({ findings: [errorFinding] }));
    render(<ValidationCountIndicator />);
    fireEvent.click(screen.getByTestId('open-validation-panel'));
    expect(useEditorStore.getState().validationPanelOpen).toBe(true);
  });

  it('debounce collapses a burst of edits into one trailing revalidation', async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({ fields: [] }),
      })),
    );
    const store = useEditorStore.getState();
    for (let i = 0; i < 5; i++) store.scheduleRevalidation();
    // Nothing has fired yet — the burst is still collapsing.
    expect(useEditorStore.getState().validateCallCount).toBe(0);

    await act(async () => {
      vi.advanceTimersByTime(500);
    });
    // Exactly one trailing validate call for the whole burst.
    expect(useEditorStore.getState().validateCallCount).toBe(1);
  });

  it('discards a stale validate response, keeping the fresher findings', async () => {
    // Fake timers keep the debounce from firing; we drive runValidation directly.
    vi.useFakeTimers();
    const resolvers: Array<(fields: Finding[]) => void> = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            resolvers.push((fields) =>
              resolve({ ok: true, status: 200, json: async () => ({ fields }) }),
            );
          }),
      ),
    );

    const aFinding: Finding = { field: 'entities.A.fields.x', code: 'c', severity: 'error', message: 'edit A' };
    const bFinding: Finding = { field: 'entities.B.fields.y', code: 'c', severity: 'error', message: 'edit B' };

    const store = useEditorStore.getState();
    store.applyModel(emptyModel()); // draft -> edit A version
    const pA = store.runValidation(); // issued for edit A
    store.applyModel(emptyModel()); // draft -> edit B version (supersedes A)
    const pB = store.runValidation(); // issued for edit B

    // Edit B resolves first and is applied.
    await act(async () => {
      resolvers[1](bFinding ? [bFinding] : []);
      await pB;
    });
    expect(useEditorStore.getState().findings).toEqual([bFinding]);

    // Edit A resolves LATER; it is stale and must be discarded, not rendered
    // over the fresher edit-B findings.
    await act(async () => {
      resolvers[0]([aFinding]);
      await pA;
    });
    expect(useEditorStore.getState().findings).toEqual([bFinding]);
  });
});
