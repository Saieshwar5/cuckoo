import { Directory, File, Paths } from 'expo-file-system';
import { useEffect, useState } from 'react';

import { useSession } from '@/session/SessionProvider';

import { swept } from './gone';

// How an <Image> or a player reaches a file on the hub, on a phone.
//
// The bytes are behind the session token, so they are never a plain link.
// They are fetched once, with the header, into the phone's cache directory
// and shown from there; the next time — and any time there is no signal —
// they are already on the phone. The system may reclaim the cache directory
// when storage is short, and then they are fetched again, which is the
// right behaviour for a copy of something the hub keeps.
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
    let cancelled = false;
    const url = api.mediaUrl(mediaId, thumb ? 'thumb' : undefined);
    void (async () => {
      try {
        const file = await fetchToCache(url, mediaId, thumb, token);
        if (!cancelled && file) setUri(file);
      } catch {
        // No copy on the phone and no way to get one now. If the hub has
        // swept it, the bubble is told; otherwise the placeholder stays
        // and the next open tries again.
        const missing = await swept(url, token);
        if (!cancelled && missing) setGone(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [api, mediaId, thumb, token, localUri]);

  // Our own send, still uploading: the file is already on this device.
  if (localUri) return { source: { uri: localUri }, gone: false };
  return { source: uri ? { uri } : null, gone };
}

// fetchToCache returns the local copy of a file, downloading it first if
// there is none.
async function fetchToCache(url: string, mediaId: string, thumb: boolean | undefined, token: string) {
  const directory = new Directory(Paths.cache, 'cuckoo', 'media');
  if (!directory.exists) directory.create({ intermediates: true });
  const file = new File(directory, `${mediaId}${thumb ? '.thumb' : ''}`);
  if (file.exists) return file.uri;
  const saved = await File.downloadFileAsync(url, file, {
    headers: { Authorization: `Bearer ${token}` },
    idempotent: true,
  });
  return saved.uri;
}

// forgetMedia removes every copy. Signing out calls it.
export function forgetMedia(): void {
  try {
    const directory = new Directory(Paths.cache, 'cuckoo', 'media');
    if (directory.exists) directory.delete();
  } catch {
    // Nothing to forget, or nowhere to forget it from.
  }
}
