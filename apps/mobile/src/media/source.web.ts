import { useEffect, useState } from 'react';

import { useSession } from '@/session/SessionProvider';

import { clearBlobs, getBlob, putBlob } from './blobs.web';
import { isGone } from './gone';

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

// What a bubble gets: the file, or not yet, or never. A file the hub has
// swept is gone for good, and a bubble should say so rather than wait.
export interface MediaState {
  source: MediaSource | null;
  gone: boolean;
}

export function useMediaSource(
  mediaId: string | undefined,
  opts: { thumb?: boolean; localUri?: string } = {},
): MediaState {
  const { api, token } = useSession();
  const { thumb, localUri } = opts;
  const [uri, setUri] = useState<string | null>(null);
  const [gone, setGone] = useState(false);

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
          if (!res.ok) {
            // Swept by the hub, which the bubble says; or something else,
            // which it does not, and the placeholder stays.
            const missing = await isGone(res);
            if (!cancelled && missing) setGone(true);
            return;
          }
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

  if (localUri) return { source: { uri: localUri }, gone: false };
  return { source: uri ? { uri } : null, gone };
}

// forgetMedia removes every copy. Signing out calls it.
export function forgetMedia(): void {
  void clearBlobs();
}
