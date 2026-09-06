import type { Api } from '@/api/client';

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
): Promise<void> {
  const res = await fetch(api.mediaUrl(mediaId), {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });
  if (!res.ok) return;
  const url = URL.createObjectURL(await res.blob());
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName || 'file';
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
