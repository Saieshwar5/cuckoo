import type { Api } from '@/api/client';

import { isGone } from './gone';

// How opening went. A file the hub has swept is said so by the card;
// any other failure is left as a click that did nothing, which is what it
// was before, and the next click tries again.
export type Opened = 'opened' | 'gone' | 'failed';

// Opening a file a chat carries, in a browser.
//
// The bytes need the session token, so a plain link to them would fail;
// they are fetched, wrapped in a blob URL, and handed to a link that the
// page clicks for itself. The URL is released straight after: it exists
// only for the moment the download starts.
export async function openAttachment(
  api: Api,
  mediaId: string,
  fileName: string,
  token: string | null,
): Promise<Opened> {
  const res = await fetch(api.mediaUrl(mediaId), {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) return (await isGone(res)) ? 'gone' : 'failed';
  const url = URL.createObjectURL(await res.blob());
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName || 'file';
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
  return 'opened';
}
