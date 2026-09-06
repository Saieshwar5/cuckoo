import * as Network from 'expo-network';

// The platform saying the network is back.
//
// A socket that dropped reconnects on a backoff, and a send that failed
// retries on another; both are right for a hub that is down. For a phone
// that just came out of a tunnel they are wrong: the network is back now,
// and everyone waiting on it should know now rather than in thirty
// seconds. The platform knows the moment it happens and says so.
export function onNetworkBack(listener: () => void): () => void {
  let wasConnected: boolean | null = null;
  try {
    const subscription = Network.addNetworkStateListener((state) => {
      const connected = state.isConnected !== false;
      if (connected && wasConnected === false) listener();
      wasConnected = connected;
    });
    return () => subscription?.remove?.();
  } catch {
    // A platform with no way to say so. The backoffs still apply.
    return () => {};
  }
}
