import { useSession } from '@/session/SessionProvider';

// How an <Image> reaches a file on the hub.
//
// The bytes are behind the session token, so they are never a plain link.
// On a phone, React Native's image loader sends headers for us; the web
// has its own version of this file, because a browser's <img> cannot.
export interface MediaSource {
  uri: string;
  headers?: Record<string, string>;
}

export function useMediaSource(
  mediaId: string | undefined,
  opts: { thumb?: boolean; localUri?: string } = {},
): MediaSource | null {
  const { api, token } = useSession();
  // Our own send, still uploading: the file is already on this device.
  if (opts.localUri) return { uri: opts.localUri };
  if (!mediaId || !token) return null;
  return {
    uri: api.mediaUrl(mediaId, opts.thumb ? 'thumb' : undefined),
    headers: { Authorization: `Bearer ${token}` },
  };
}
