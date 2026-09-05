import en from './en.json';

type Params = Record<string, string | number>;

const strings: Record<string, string> = en;

// t looks a string up by key and fills {placeholders}. English is the only
// language shipped; another is another JSON file. A missing key returns the
// key itself, so a typo is visible on screen rather than blank.
export function t(key: string, params?: Params): string {
  const template = strings[key] ?? key;
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_, name: string) =>
    name in params ? String(params[name]) : `{${name}}`,
  );
}
