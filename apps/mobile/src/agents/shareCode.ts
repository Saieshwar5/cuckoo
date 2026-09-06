import * as SecureStore from 'expo-secure-store';

// The code behind an agent's Share screen. The hub shows a code once, so
// the phone keeps it, in the keychain: anyone holding it can add the agent.
const key = (agentId: string) => `share.${agentId.replace(/[^A-Za-z0-9._-]/g, '_')}`;

export async function loadShareCode(agentId: string): Promise<{ tokenId: string; code: string } | null> {
  try {
    const raw = await SecureStore.getItemAsync(key(agentId));
    return raw ? (JSON.parse(raw) as { tokenId: string; code: string }) : null;
  } catch {
    return null;
  }
}

export async function saveShareCode(
  agentId: string,
  value: { tokenId: string; code: string },
): Promise<void> {
  await SecureStore.setItemAsync(key(agentId), JSON.stringify(value));
}

export async function clearShareCode(agentId: string): Promise<void> {
  await SecureStore.deleteItemAsync(key(agentId));
}
