// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: done-control
// parlay-artifact: test
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { DoneControl } from './DoneControl';
import { useEditorStore } from '../../store/editorStore';
import { populatedModel, populatedEnvelope, clone } from '../../test/fixtures';

function stubFetch(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({ ok: true, status: 200, json: async () => body })),
  );
}

beforeEach(() => {
  useEditorStore.getState().resetStore();
  useEditorStore.getState().hydrate(clone(populatedEnvelope));
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('DoneControl', () => {
  it('prompts save-or-discard when Done is pressed with a dirty draft — no shutdown yet', () => {
    act(() => useEditorStore.getState().applyModel(clone(populatedModel)));
    const onShutdown = vi.fn();
    render(<DoneControl onShutdown={onShutdown} />);

    fireEvent.click(screen.getByTestId('done-button'));

    expect(screen.getByTestId('dirty-done-prompt')).toBeInTheDocument();
    expect(onShutdown).not.toHaveBeenCalled();
  });

  it('save-and-finish persists the draft before shutting down', async () => {
    stubFetch({ model: populatedModel, etag: 'sha256:saved' });
    act(() => useEditorStore.getState().applyModel(clone(populatedModel)));
    const onShutdown = vi.fn();
    render(<DoneControl onShutdown={onShutdown} />);

    fireEvent.click(screen.getByTestId('done-button'));
    fireEvent.click(screen.getByTestId('confirm-save-and-finish'));

    await waitFor(() => expect(onShutdown).toHaveBeenCalledTimes(1));
    // The save landed (fresh etag) before the shutdown fired.
    expect(useEditorStore.getState().etag).toBe('sha256:saved');
    expect(useEditorStore.getState().isDirty).toBe(false);
  });
});
