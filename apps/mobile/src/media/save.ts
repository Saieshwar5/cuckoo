import { Directory, File, Paths } from 'expo-file-system';
import * as Sharing from 'expo-sharing';

import type { Api } from '@/api/client';

// Opening a file a chat carries, on a phone.
//
// The bytes are behind the session token, so they are downloaded here with
// it and handed to the system's own share sheet, which is where a phone
// decides what opens a PDF. Saving into the cache is deliberate: this is a
// copy of something that lives on the hub, and the system may reclaim it.
export async function openAttachment(
  api: Api,
  mediaId: string,
  fileName: string,
  token: string | null,
): Promise<void> {
  const directory = new Directory(Paths.cache, 'cuckoo');
  if (!directory.exists) directory.create({ intermediates: true });

  const file = await File.downloadFileAsync(api.mediaUrl(mediaId), new File(directory, fileName), {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    idempotent: true,
  });
  if (await Sharing.isAvailableAsync()) {
    await Sharing.shareAsync(file.uri);
  }
}
