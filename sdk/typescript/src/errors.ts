/**
 * The one error shape the hub answers with, and how a refusal becomes it.
 */

/**
 * The hub refused an operation. `code` is the stable identifier to match on;
 * `message` is the sentence written for a person to read.
 *
 * Codes met most often: `no_binding`, `not_participant`, `blocked`,
 * `invalid_text`, `not_streaming`, `rate_limited`.
 */
export class ProtocolError extends Error {
  readonly code: string;
  /** The field at fault, when the hub named one. */
  readonly field?: string;
  /** The hub's identifier for this request; quote it when asking for help. */
  readonly requestId?: string;
  readonly status: number;

  constructor(
    code: string,
    message: string,
    options: { field?: string; requestId?: string; status?: number } = {},
  ) {
    super(`${code}: ${message}`);
    this.name = "ProtocolError";
    this.code = code;
    this.field = options.field;
    this.requestId = options.requestId;
    this.status = options.status ?? 0;
  }
}

/**
 * Turn a refusal into the hub's own error, so a caller sees `file_too_large`
 * rather than `422 Unprocessable Entity`.
 *
 * The body is read here, which consumes the response — callers that want the
 * bytes must check `response.ok` themselves first.
 */
export async function raiseForStatus(response: Response): Promise<void> {
  if (response.ok) return;

  let code = "http_error";
  let message = `${response.status}`;
  let field: string | undefined;
  let requestId: string | undefined;

  const text = await response.text().catch(() => "");
  try {
    const error = (JSON.parse(text) as { error?: Record<string, string> }).error;
    if (error) {
      code = error.code ?? code;
      message = error.message ?? message;
      field = error.field;
      requestId = error.request_id;
    }
  } catch {
    // A hub — or something in front of it — that answered with prose. Keep a
    // little of it: "502: <html>…" says more than "http_error".
    message = `${response.status}: ${text.slice(0, 200)}`;
  }

  throw new ProtocolError(code, message, { field, requestId, status: response.status });
}
