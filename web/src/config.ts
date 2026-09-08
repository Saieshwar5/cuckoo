// Everything about the site that is a decision rather than a fact, in one
// place, so changing the domain is one edit and not a search.
//
// The domain is cuckoo.onl. `SITE_URL` in the environment wins, so a preview
// deploy can run on its own host without a commit.

/** Where the site is served. The hub answers on the same host under /v1, /p and /a. */
export const siteUrl = import.meta.env.SITE_URL ?? 'https://cuckoo.onl';

/** The hub every code sample talks to. Same host as the site, by design. */
export const hubUrl = siteUrl;

export const site = {
  name: 'Cuckoo',
  tagline: 'A messenger where AI agents are first-class.',
  description:
    'Cuckoo is a chat app built for AI agents. Companies and individuals ' +
    'connect their own backend over an open protocol. The hub only moves ' +
    'messages: it never runs a model.',
  repo: 'https://github.com/Saieshwar5/cuckoo',
  /** One address for support, privacy questions and, in time, grievances. */
  contact: 'hello@cuckoo.onl',
  /** The protocol's own name, so a company can say what it supports. */
  protocol: { name: 'Cuckoo Agent Protocol', short: 'CAP', version: '0.1' },
} as const;

/** Where the app can be got today. Store links replace these once review passes. */
export const downloads = {
  android: { available: false, url: `${site.repo}/releases/latest`, label: 'Download the APK' },
  ios: { available: false, url: '', label: 'TestFlight' },
  play: { available: false, url: '' },
  appStore: { available: false, url: '' },
} as const;

/**
 * The agent a first-time visitor can talk to before installing anything.
 * Empty until the welcome agent is published on the live hub.
 */
export const tryAgent = { handle: '', pairUrl: '' } as const;
