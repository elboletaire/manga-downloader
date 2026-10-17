import {
  createContext,
  createElement,
  Fragment,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import {
  CATALOG,
  createT,
  resolveLanguage,
  type Catalog,
  type Lang,
  type LangPref,
  type T as Translator,
  type Vars,
} from './index';

export interface I18n {
  t: Translator;
  /** The language shown: the preference, or the system's. */
  lang: Lang;
  pref: LangPref;
  /** The language the system asks for (English when none is supported). */
  systemLang: Lang;
  /** Switches at once and saves; reverts and rejects when saving fails. */
  setLanguage: (pref: LangPref) => Promise<void>;
}

const fallback: I18n = {
  t: createT('en'),
  lang: 'en',
  pref: '',
  systemLang: 'en',
  setLanguage: async () => {},
};

const I18nContext = createContext<I18n>(fallback);

/** English outside an I18nProvider, so components render the same in tests. */
export function useI18n(): I18n {
  return useContext(I18nContext);
}

export interface I18nProviderProps {
  /** The saved preference; a change from outside takes effect too. */
  pref: LangPref;
  persist: (pref: LangPref) => Promise<void>;
  /** Tests only: the catalog and the system's languages. */
  catalog?: Catalog;
  systemLanguages?: readonly string[];
  children?: ReactNode;
}

export function I18nProvider({ pref: saved, persist, catalog = CATALOG, systemLanguages, children }: I18nProviderProps) {
  const [pref, setPref] = useState<LangPref>(saved);
  useEffect(() => {
    setPref(saved);
  }, [saved]);

  const languages = systemLanguages ?? (typeof navigator === 'undefined' ? [] : navigator.languages);
  const lang = resolveLanguage(pref, languages);
  const systemLang = resolveLanguage('', languages);

  useEffect(() => {
    if (typeof document !== 'undefined') document.documentElement.lang = lang;
  }, [lang]);

  const setLanguage = useCallback(
    async (next: LangPref) => {
      const previous = pref;
      setPref(next);
      try {
        await persist(next);
      } catch (err) {
        setPref(previous);
        throw err;
      }
    },
    [pref, persist],
  );

  const value = useMemo<I18n>(
    () => ({ t: createT(lang, catalog), lang, pref, systemLang, setLanguage }),
    [lang, catalog, pref, systemLang, setLanguage],
  );
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

/** Renders a rich-text tag's children, e.g. `<code>` around a flag. */
export type Components = Record<string, (children: ReactNode) => ReactNode>;

const TAGS: Components = {
  code: (children) => createElement('code', null, children),
  em: (children) => createElement('em', null, children),
  kbd: (children) => createElement('kbd', null, children),
  strong: (children) => createElement('strong', null, children),
};

/** Turns the message's known tags into elements; other text, tags included, stays as it is. */
export function richText(text: string, components: Components): ReactNode[] {
  let seq = 0;
  const key = () => seq++;
  const root: { tag: string; children: ReactNode[] } = { tag: '', children: [] };
  const stack = [root];
  const add = (frame: { children: ReactNode[] }, node: ReactNode) => {
    frame.children.push(typeof node === 'string' ? node : createElement(Fragment, { key: key() }, node));
  };

  for (const part of text.split(/(<\/?[a-z][a-z0-9]*>)/i)) {
    const top = stack[stack.length - 1];
    const open = /^<([a-z][a-z0-9]*)>$/i.exec(part)?.[1].toLowerCase();
    const close = /^<\/([a-z][a-z0-9]*)>$/i.exec(part)?.[1].toLowerCase();
    if (open !== undefined && Object.hasOwn(components, open)) {
      stack.push({ tag: open, children: [] });
    } else if (close !== undefined && stack.length > 1 && top.tag === close) {
      stack.pop();
      add(stack[stack.length - 1], components[close](top.children));
    } else if (part !== '') {
      top.children.push(part);
    }
  }
  // An unclosed tag keeps its text, without the tag.
  while (stack.length > 1) {
    const frame = stack.pop()!;
    stack[stack.length - 1].children.push(...frame.children);
  }
  return root.children;
}

/** Rich text: `<T k="app.hint" values={{ key: 'Ctrl+,' }} />`, tags from `components` or TAGS. */
export function T({ k, values, components }: { k: string; values?: Vars; components?: Components }): ReactNode {
  const { t } = useI18n();
  return richText(t(k, values), { ...TAGS, ...components });
}
