import { Linking } from 'react-native';

// openLink hands an address to the phone and steps out of the way: a page
// to the browser, a upi: link to whichever payment app answers for it, a
// tel: link to the dialler. Cuckoo opens nothing itself and shows nothing
// in a frame of its own — a link leaves, visibly, which is the whole
// bargain of putting one in a chat with an agent nobody has verified.
export async function openLink(url: string): Promise<void> {
  try {
    await Linking.openURL(url);
  } catch {
    // Nothing on this device answers for that kind of address — a upi:
    // link on a laptop, say. There is nowhere to go, so we stay.
  }
}
