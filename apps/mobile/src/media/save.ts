import { Directory, File, Paths } from 'expo-file-system';
import * as Sharing from 'expo-sharing';

import type { Api } from '@/api/client';

import { swept } from './gone';

// How opening went. A file the hub has swept is said so by the card;
// any other failure is left as a tap that did nothing, which is what it
// was before, and the next tap tries again.
export type Opened = 'opened' | 'gone' | 'failed';

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
): Promise<Opened> {
  const directory = new Directory(Paths.cache, 'cuckoo');
  if (!directory.exists) directory.create({ intermediates: true });

  const url = api.mediaUrl(mediaId);
  let file: File;
  try {
    file = await File.downloadFileAsync(url, new File(directory, fileName), {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      idempotent: true,
    });
  } catch {
    return token && (await swept(url, token)) ? 'gone' : 'failed';
  }
  if (await Sharing.isAvailableAsync()) {
    await Sharing.shareAsync(file.uri);
  }
  return 'opened';
}
