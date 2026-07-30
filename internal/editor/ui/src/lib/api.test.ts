// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
// parlay-artifact: test
import { loadModel, shutdownSession } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('shutdownSession', () => {
  // The regression this exists for: the Done control called a handler that
  // only flipped a local flag. The served bundle contained no reference to
  // /api/shutdown at all, so the overlay appeared, the server kept serving,
  // and `parlay domain-edit` — a blocking hook whose process exit is the
  // completion signal — stayed blocked until its 30-minute idle timeout.
  it('POSTs to the shutdown endpoint', async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, status: 202, json: async () => ({}) }));
    vi.stubGlobal('fetch', fetchMock);

    await shutdownSession();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/shutdown');
    expect(init.method).toBe('POST');
  });

  // The server tears its listener down as it shuts down, so the in-flight
  // request being cut off IS the success path. Surfacing that as an error
  // would put a failure banner on the one action that worked.
  it('never throws when the connection is cut by the shutdown it asked for', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('Failed to fetch');
      }),
    );

    await expect(shutdownSession()).resolves.toBeUndefined();
  });
});

describe('loadModel', () => {
  // Defence in depth for the blank-page bug. The server normalises these
  // now, but the envelope cast is unchecked: a null collection reaching the
  // store makes every .length/.map in the editor throw during render, which
  // unmounts the page to a blank screen with the error only in the console.
  it('normalises null collections to empty arrays', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({
          model: { schema_version: 1, enums: null, entities: null },
          etag: 'sha256:x',
        }),
      })),
    );

    const env = await loadModel();

    expect(env.model.enums).toEqual([]);
    expect(env.model.entities).toEqual([]);
  });

  it('leaves populated collections alone', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({
          model: {
            schema_version: 1,
            enums: [{ name: 'Role', values: [{ value: 'admin' }] }],
            entities: [{ name: 'Widget', fields: [] }],
          },
          etag: 'sha256:y',
        }),
      })),
    );

    const env = await loadModel();

    expect(env.model.enums).toHaveLength(1);
    expect(env.model.entities).toHaveLength(1);
  });
});
