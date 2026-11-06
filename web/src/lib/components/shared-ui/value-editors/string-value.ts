import type { Json } from '$lib/wpi/types';

export type StringMode = 'single' | 'multilingual' | 'list';

export function stringLanguages(languages: string[] | undefined): string[] {
 return languages?.length ? languages : ['en', 'zh-cn'];
}

// A language map is shown in Multilingual; any other value in the Single or List mode chosen.
export function currentStringMode(value: Json | undefined, chosen: StringMode | undefined): StringMode {
 if (value !== null && typeof value === 'object' && !Array.isArray(value)) return 'multilingual';
 return chosen === 'list' ? 'list' : 'single';
}

export function emptyStringValue(value: string, emptyAsNull: boolean): string | null {
 return value === '' && emptyAsNull ? null : value;
}

export function convertStringMode(value: Json | undefined, mode: StringMode, languages: string[], locale: string): Json {
 const map = value !== null && typeof value === 'object' && !Array.isArray(value) ? value : null;
 if (mode === 'multilingual') {
  if (map) return { ...map };
  return typeof value === 'string' ? { [languages.includes(locale) ? locale : languages[0]]: value } : {};
 }
 if (!map) return typeof value === 'string' ? value : null;
 const current = map[locale];
 if (typeof current === 'string') return current;
 for (const language of languages) if (typeof map[language] === 'string') return map[language];
 return null;
}

export function warnsAboutTranslations(value: Record<string, Json>, languages: string[], locale: string, mode: StringMode): boolean {
 if (mode !== 'list') return false;
 const selected = typeof value[locale] === 'string' ? locale : languages.find(language => typeof value[language] === 'string');
 return Object.entries(value).some(([language, text]) => language !== selected && typeof text === 'string' && text !== '');
}
