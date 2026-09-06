import { hubUrl } from '@/config';

import type { MediaSource } from './source';

// Where an agent's picture lives.
//
// It is the one public address in Cuckoo, so it needs no token and no
// fetching dance: a plain image source, which works the same in a browser
// and on a phone. A person's photo is not like this — it goes through
// useMediaSource, behind their session.
export function agentAvatar(agentId: string, hasAvatar: boolean | undefined): MediaSource | null {
  return hasAvatar ? { uri: `${hubUrl}/a/${agentId}/avatar` } : null;
}
