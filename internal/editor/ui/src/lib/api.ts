// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
// parlay-extends: domain-model-editor/domain-model-editor-validation/cross-cutting/validation-surfacing-integration
import type { DomainModelDocument } from '../types/domain';

export interface ModelEnvelope {
  model: DomainModelDocument;
  etag: string;
}

/** Finding severity — the closed set Core emits. */
export type FindingSeverity = 'error' | 'warning';

/**
 * One validation finding as returned by the validate endpoint: the element
 * path (`field`, the distinguished top-level token for a whole-model finding),
 * the closed error `code`, the Core-emitted `severity`, and the human
 * `message` / actionable `fix`. Studio renders these verbatim.
 */
export interface Finding {
  field: string;
  code: string;
  severity: FindingSeverity;
  message: string;
  fix?: string;
}

/** The distinguished element path a whole-model finding carries. */
export const WHOLE_MODEL_PATH = '<domain-model>';

export interface FieldError {
  field: string;
  message: string;
}

export type ApiErrorCode =
  | 'conflict'
  | 'validation-failed'
  | 'server-error'
  | 'network-error';

export class ApiError extends Error {
  code: ApiErrorCode;
  currentEtag?: string;
  attemptedEtag?: string;
  fields?: FieldError[];
  requestId?: string;

  constructor(
    code: ApiErrorCode,
    init: {
      message?: string;
      currentEtag?: string;
      attemptedEtag?: string;
      fields?: FieldError[];
      requestId?: string;
    } = {},
  ) {
    super(init.message ?? code);
    this.name = 'ApiError';
    this.code = code;
    this.currentEtag = init.currentEtag;
    this.attemptedEtag = init.attemptedEtag;
    this.fields = init.fields;
    this.requestId = init.requestId;
  }
}

const MODEL_URL = '/api/domain-model/model';
const VALIDATE_URL = '/api/domain-model/validate';
const SHUTDOWN_URL = '/api/shutdown';

interface ValidateEnvelope {
  fields: Finding[];
}

interface ConflictEnvelope {
  code: 'conflict';
  current_etag: string;
  attempted_etag: string;
}

interface ValidationEnvelope {
  code: 'validation-failed';
  fields: FieldError[];
}

interface ServerErrorEnvelope {
  code: 'server-error';
  request_id: string;
}

type ErrorEnvelope =
  | ConflictEnvelope
  | ValidationEnvelope
  | ServerErrorEnvelope
  | { code?: string };

function throwFromEnvelope(env: ErrorEnvelope): never {
  switch (env.code) {
    case 'conflict': {
      const e = env as ConflictEnvelope;
      throw new ApiError('conflict', {
        currentEtag: e.current_etag,
        attemptedEtag: e.attempted_etag,
        message: 'The model changed on disk since you loaded it.',
      });
    }
    case 'validation-failed': {
      const e = env as ValidationEnvelope;
      throw new ApiError('validation-failed', {
        fields: e.fields,
        message: 'The model failed validation.',
      });
    }
    case 'server-error': {
      const e = env as ServerErrorEnvelope;
      throw new ApiError('server-error', {
        requestId: e.request_id,
        message: 'The server encountered an error.',
      });
    }
    default:
      throw new ApiError('server-error', {
        message: 'Unexpected error response.',
      });
  }
}

export async function loadModel(): Promise<ModelEnvelope> {
  let res: Response;
  try {
    res = await fetch(MODEL_URL, {
      method: 'GET',
      headers: { Accept: 'application/json' },
    });
  } catch (err) {
    throw new ApiError('network-error', {
      message: err instanceof Error ? err.message : 'Network request failed.',
    });
  }
  if (!res.ok) {
    throwFromEnvelope(await res.json().catch(() => ({})));
  }
  return normalizeEnvelope((await res.json()) as ModelEnvelope);
}

/**
 * Defence in depth against a null collection arriving on the wire.
 *
 * The server normalises these (see decodeAndMigrate in the domain loader),
 * and DomainModelDocument types `enums` and `entities` as non-optional
 * arrays — but the cast that produces the envelope is unchecked, so a server
 * that regressed would put `null` straight into the store. Every `.length`
 * and `.map` in the editor would then throw during render and unmount the
 * page, which is a blank screen with the error visible only in the browser
 * console. Cheap to prevent at the one boundary every model passes through.
 *
 * Same idiom as `env.fields ?? []` in validateModel.
 */
function normalizeEnvelope(env: ModelEnvelope): ModelEnvelope {
  return {
    ...env,
    model: {
      ...env.model,
      enums: env.model?.enums ?? [],
      entities: env.model?.entities ?? [],
    },
  };
}

/**
 * Validate a draft out of process via POST /api/domain-model/validate. It
 * returns the complete finding list for a well-formed draft (an empty array
 * when clean) and surfaces a malformed-request `validation-failed` as an
 * ApiError, distinct from a finding list. Follows the thin-fetch-client +
 * typed error-envelope convention the mvp established.
 */
export async function validateModel(
  model: DomainModelDocument,
): Promise<Finding[]> {
  let res: Response;
  try {
    res = await fetch(VALIDATE_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({ model }),
    });
  } catch (err) {
    throw new ApiError('network-error', {
      message: err instanceof Error ? err.message : 'Network request failed.',
    });
  }
  if (!res.ok) {
    // A malformed request is the one HTTP-error case (validation-failed);
    // a well-formed-but-invalid draft is a 200 finding list, never a 4xx.
    throwFromEnvelope(await res.json().catch(() => ({})));
  }
  const env = (await res.json()) as ValidateEnvelope;
  return env.fields ?? [];
}

/**
 * End the editing session by asking the server to shut down, via
 * POST /api/shutdown.
 *
 * This is what makes the Done control mean anything. The control used to call
 * a handler that only flipped a local `sessionEnded` flag: the overlay
 * appeared, the server kept serving, and the process stayed blocked until its
 * idle timeout — 30 minutes by default. `parlay domain-edit` is designed as a
 * blocking hook whose process exit is the completion signal, so an agent that
 * told the user "click Done when you're finished" waited out the timeout no
 * matter what the user did.
 *
 * Deliberately never throws. The server tears the listener down as it shuts
 * down, so the in-flight request being cut off is the SUCCESS path here, not
 * an error — a rejected fetch and a 202 mean the same thing to the caller.
 * Surfacing that as an error would put a failure banner on the one action
 * that worked.
 */
export async function shutdownSession(): Promise<void> {
  try {
    await fetch(SHUTDOWN_URL, {
      method: 'POST',
      headers: { Accept: 'application/json' },
    });
  } catch {
    // Connection closed by the shutdown we just asked for. Expected.
  }
}

export async function saveModel(
  model: DomainModelDocument,
  etag: string,
): Promise<ModelEnvelope> {
  let res: Response;
  try {
    res = await fetch(MODEL_URL, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify({ model, etag }),
    });
  } catch (err) {
    throw new ApiError('network-error', {
      message: err instanceof Error ? err.message : 'Network request failed.',
    });
  }
  if (!res.ok) {
    throwFromEnvelope(await res.json().catch(() => ({})));
  }
  // Normalised for the same reason as loadModel: the save handler echoes the
  // model back, so a null collection sent up round-trips straight into the
  // store.
  return normalizeEnvelope((await res.json()) as ModelEnvelope);
}
