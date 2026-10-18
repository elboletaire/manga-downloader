import {
  createContext,
  createElement,
  Fragment,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import {
  CATALOG,
  createT,
  createTemplate,
  fill,
  resolveLanguage,
  type Catalog,
  type Lang,
  type LangPref,
  type T as Translator,
  type Template,
  type Vars,
} from './index';

export interface I18n {
  t: Translator;
  /** The message as written, for T's rich text. */
  template: Template;
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
  template: createTemplate('en'),
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

/**
 * Applies `next` at once and saves it. When saving fails, reverts to `previous`
 * and rethrows, unless `isLatest` says a newer switch has started since.
 */
export async function switchLanguage(
  previous: LangPref,
  next: LangPref,
  apply: (pref: LangPref) => void,
  persist: (pref: LangPref) => Promise<void>,
  isLatest: () => boolean = () => true,
): Promise<void> {
  apply(next);
  try {
    await persist(next);
  } catch (err) {
    if (isLatest()) apply(previous);
    throw err;
  }
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

  const latest = useRef(0);
  const languages = systemLanguages ?? (typeof navigator === 'undefined' ? [] : navigator.languages);
  const lang = resolveLanguage(pref, languages);
  const systemLang = resolveLanguage('', languages);

  useEffect(() => {
    if (typeof document !== 'undefined') document.documentElement.lang = lang;
  }, [lang]);

  const setLanguage = useCallback(
    (next: LangPref) => {
      const ticket = ++latest.current;
      return switchLanguage(pref, next, setPref, persist, () => ticket === latest.current);
    },
    [pref, persist],
  );

  const value = useMemo<I18n>(
    () => ({ t: createT(lang, catalog), template: createTemplate(lang, catalog), lang, pref, systemLang, setLanguage }),
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

/**
 * Turns the message's known tags into elements; other text, tags included, stays as it is.
 * The tags are split off before `vars` fill the text, so a value is never read as markup.
 */
export function richText(text: string, components: Components, vars: Vars = {}): ReactNode[] {
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
      top.children.push(fill(part, vars));
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
  const { template } = useI18n();
  return richText(template(k, values), { ...TAGS, ...components }, values);
}
