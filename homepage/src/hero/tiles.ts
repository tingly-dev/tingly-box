// Pre-rendered tiles for the hero canvas: brand icons on white cards, and the
// Tingly Box "T" icon the assembly starts from and folds back into.

export interface Palette {
  brandBg: string;
  brandFg: string;
}

export function makeCanvas(w: number, h = w): HTMLCanvasElement {
  const c = document.createElement('canvas');
  c.width = w;
  c.height = h;
  return c;
}

export function ctx2d(c: HTMLCanvasElement): CanvasRenderingContext2D {
  const g = c.getContext('2d');
  if (!g) throw new Error('2d canvas unavailable');
  return g;
}

function roundRect(g: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number): void {
  g.beginPath();
  g.roundRect(x, y, w, h, r);
}

const S = 160;

export function iconTile(img: HTMLImageElement | null): HTMLCanvasElement {
  const c = makeCanvas(S);
  const g = ctx2d(c);
  roundRect(g, 2, 2, S - 4, S - 4, S * 0.24);
  g.fillStyle = '#ffffff';
  g.fill();
  g.lineWidth = 3;
  g.strokeStyle = 'rgba(20, 24, 33, 0.10)';
  g.stroke();
  if (img) {
    const s = S * 0.6;
    g.drawImage(img, (S - s) / 2, (S - s) / 2, s, s);
  }
  return c;
}

/** The Tingly Box icon: rounded square with a "T". */
export function tTile(bg: string, fg: string, size = S): HTMLCanvasElement {
  const c = makeCanvas(size);
  const g = ctx2d(c);
  roundRect(g, size * 0.02, size * 0.02, size * 0.96, size * 0.96, size * 0.3);
  g.fillStyle = bg;
  g.fill();
  const hi = g.createLinearGradient(0, 0, 0, size);
  hi.addColorStop(0, 'rgba(255,255,255,0.10)');
  hi.addColorStop(1, 'rgba(255,255,255,0)');
  g.fillStyle = hi;
  g.fill();
  g.fillStyle = fg;
  g.font = `700 ${size * 0.54}px Inter, system-ui, sans-serif`;
  g.textAlign = 'center';
  g.textBaseline = 'middle';
  g.fillText('T', size / 2, size / 2 + size * 0.04);
  return c;
}

/**
 * Loads an SVG for canvas use. Lobehub icons declare width="1em", which some
 * browsers rasterise at 16px when drawn onto a canvas, so give them a real size.
 */
export async function loadSvg(url: string): Promise<HTMLImageElement | null> {
  try {
    const text = await (await fetch(url)).text();
    const sized = text.replace(/<svg([^>]*)>/, (_m, attrs: string) => {
      // Vite may inline small SVGs as data URIs with single-quoted attributes.
      const clean = attrs.replace(/\s(width|height)=(["'])[^"']*\2/g, '');
      return `<svg${clean} width="128" height="128">`;
    });
    const blobUrl = URL.createObjectURL(new Blob([sized], { type: 'image/svg+xml' }));
    const img = new Image();
    img.src = blobUrl;
    await img.decode();
    return img;
  } catch {
    return null;
  }
}
