import { useEffect, useState } from 'react';

import { useSession } from '@/session/SessionProvider';

import { clearBlobs, getBlob, putBlob } from './blobs.web';

// The browser's version of reaching a file on the hub.
//
// An <img> tag cannot carry an Authorization header, and the bytes are not
// public, so they are fetched here and handed to the tag as a blob URL.
// Each one is released when it is no longer on screen; leaving them behind
// keeps the whole picture in memory for as long as the tab is open.
export interface MediaSource {
  uri: string;
  headers?: Record<string, string>;
}

export function useMediaSource(
  mediaId: string | undefined,
  opts: { thumb?: boolean; localUri?: string } = {},
): MediaSource | null {
  const { api, token } = useSession();
  const { thumb, localUri } = opts;
  const [uri, setUri] = useState<string | null>(null);

  useEffect(() => {
    if (localUri || !mediaId || !token) return;
    let objectUrl: string | null = null;
    let cancelled = false;

    const key = `${mediaId}${thumb ? ':thumb' : ''}`;
    void (async () => {
      // What this browser fetched before is shown first and fastest; with
      // no signal it is all that is shown, which is the point of keeping it.
      let blob = await getBlob(key);
      if (!blob) {
        try {
          const res = await fetch(api.mediaUrl(mediaId, thumb ? 'thumb' : undefined), {
            headers: { Authorization: `Bearer ${token}` },
          });
          if (!res.ok) return;
          blob = await res.blob();
          void putBlob(key, blob);
        } catch {
          // A picture that will not load is a broken bubble, not a broken
          // chat: the placeholder stays and the rest of the screen works.
          return;
        }
      }
      if (cancelled) return;
      objectUrl = URL.createObjectURL(blob);
      setUri(objectUrl);
    })();

    return () => {
      cancelled = true;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [api, mediaId, thumb, token, localUri]);

  if (localUri) return { uri: localUri };
  return uri ? { uri } : null;
}

// forgetMedia removes every copy. Signing out calls it.
export function forgetMedia(): void {
  void clearBlobs();
}
