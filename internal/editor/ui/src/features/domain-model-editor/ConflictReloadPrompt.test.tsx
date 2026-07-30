// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: conflict-reload-prompt
// parlay-artifact: test
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { ConflictReloadPrompt } from './ConflictReloadPrompt';
import { useEditorStore } from '../../store/editorStore';
import {
  populatedModel,
  populatedEnvelope,
  clone,
  LOAD_ETAG,
  CURRENT_ETAG,
} from '../../test/fixtures';

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

describe('ConflictReloadPrompt', () => {
  it('presents reload-and-reapply and keep-my-draft on a 409, never a silent overwrite', () => {
    act(() =>
      useEditorStore.setState({
        conflict: { currentEtag: CURRENT_ETAG, attemptedEtag: LOAD_ETAG },
      }),
    );
    render(<ConflictReloadPrompt />);

    expect(screen.getByTestId('conflict-explanation')).toBeInTheDocument();
    expect(screen.getByTestId('reload-and-reapply')).toBeInTheDocument();
    expect(screen.getByTestId('keep-my-draft')).toBeInTheDocument();
  });

  it('reload refetches the model with a fresh etag and fires no automatic re-save', async () => {
    stubFetch({ model: populatedModel, etag: CURRENT_ETAG });
    act(() =>
      useEditorStore.setState({
        conflict: { currentEtag: CURRENT_ETAG, attemptedEtag: LOAD_ETAG },
      }),
    );
    render(<ConflictReloadPrompt />);

    fireEvent.click(screen.getByTestId('reload-and-reapply'));

    await waitFor(() =>
      expect(useEditorStore.getState().etag).toBe(CURRENT_ETAG),
    );
    // The prompt dismissed and no re-save was issued (a single GET only).
    expect(useEditorStore.getState().conflict).toBeNull();
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});
