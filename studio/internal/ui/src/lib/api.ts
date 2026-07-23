// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
import type { DomainModelDocument } from '../types/domain';

export interface ModelEnvelope {
  model: DomainModelDocument;
  etag: string;
}

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
  return (await res.json()) as ModelEnvelope;
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
  return (await res.json()) as ModelEnvelope;
}
