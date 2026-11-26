import { useEffect, useId } from 'react';

// The browser tab's title: the most specific focus first, then the product and
// the site (host) it is served from, so tabs of several instances stay apart.
// Several parts of the page can name the title at once (the shell its page, a
// page its open item); the highest priority wins, the latest breaking ties.

export type TitlePart = string | number | null | undefined | false;

interface Entry {
  parts: string[];
  priority: number;
  order: number;
}

const MAX_PART = 40;
const entries = new Map<string, Entry>();
let order = 0;
let product: string | undefined;

const isDesktopOrigin = () => window.location.protocol === 'wails:' || window.location.hostname === 'wails.localhost';

export function truncatePart(part: string, max = MAX_PART): string {
  const chars = Array.from(part.trim().replace(/\s+/g, ' '));
  return chars.length > max ? `${chars.slice(0, max - 1).join('')}…` : chars.join('');
}

export function composeTitle(parts: string[], opts: { product: string; site?: string }): string {
  const tail = [opts.product, opts.site].filter(Boolean) as string[];
  return [...parts, ...tail].join(' · ');
}

function apply() {
  // index.html's own <title> names the product; read it once, before the first change.
  product ??= document.title || 'Tingly Box';
  const top = [...entries.values()].sort((a, b) => b.priority - a.priority || b.order - a.order)[0];
  document.title = composeTitle(top?.parts ?? [], {
    product,
    site: isDesktopOrigin() ? undefined : window.location.host,
  });
}

/**
 * Names the tab. `parts` run most specific first; falsy ones are skipped and
 * long ones cut. Pass a higher `priority` from a page than from the shell.
 */
export function useDocumentTitle(parts: TitlePart[], priority = 1) {
  const id = useId();
  const clean = parts
    .filter((p) => p !== null && p !== undefined && p !== false && `${p}`.trim() !== '')
    .map((p) => truncatePart(`${p}`));
  const key = JSON.stringify(clean);
  useEffect(() => {
    entries.set(id, { parts: JSON.parse(key) as string[], priority, order: ++order });
    apply();
    return () => {
      entries.delete(id);
      apply();
    };
  }, [id, key, priority]);
}
