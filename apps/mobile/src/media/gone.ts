// Whether a file the hub was asked for is one it no longer has.
//
// The hub sweeps files past its retention window and answers a request
// for one with 404 and the code media_not_found. That is not a failure to
// retry or an error to show in red: the bubble says the file is no longer
// available and leaves it at that. Anything else — a dropped connection, a
// hub in trouble — is left alone, and the bubble keeps waiting the way it
// always did.
export async function isGone(res: Pick<Response, 'status' | 'json'>): Promise<boolean> {
  if (res.status !== 404) return false;
  try {
    const body = (await res.json()) as { error?: { code?: string } } | null;
    return body?.error?.code === 'media_not_found';
  } catch {
    return false;
  }
}

// swept asks the hub itself. On a phone the download reports a refused
// response as a sentence rather than a status, so after one fails the hub
// is asked once more, and its answer is what decides. A 404's body is only
// the envelope, so the second ask costs nothing worth counting.
export async function swept(url: string, token: string): Promise<boolean> {
  try {
    return await isGone(await fetch(url, { headers: { Authorization: `Bearer ${token}` } }));
  } catch {
    return false;
  }
}
