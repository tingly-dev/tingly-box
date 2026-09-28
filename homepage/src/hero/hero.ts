// Hero animation: agents and providers assemble into the Tingly Box "T".
//
// Growth starts from the Tingly Box icon. Walkers — requests — hop between free
// neighbouring cells of a T-shaped grid; every cell they land on flips in as an
// agent (left), IM channel (middle) or model provider (right). The camera
// starts close on the icon and pulls back as the T grows; once the T is
// complete it folds back into the icon while the camera pushes in again, and
// assembly restarts from there with a different mix. After a few rounds the
// hero rests on the finished T and stops drawing until it is clicked.
//
// Idea adapted from the recursive hero on anthropic.com/institute; the
// implementation here is independent.
import { AGENTS, CHANNELS, PROVIDERS } from '../data/brands';
import { CENTER, N, START, cells as shapeCells, col, neighbours, row, side } from './shape';
import { ctx2d, iconTile, loadSvg, makeCanvas, tTile, type Palette } from './tiles';

const TILE = 0.88;                  // tile edge as a fraction of the cell pitch
const SPAN = N - 1 + TILE;          // T edge measured in pitches
const TICK = 230;                   // ms per walker step
const POP = 620;                    // ms tile pop-in
const FADE = 420;                   // ms walker fade in / out
const HOLD = 2600;                  // ms the finished T rests before folding
const FOLD = 2200;                  // ms for the T to drain back into the icon
const VARIANTS = 6;                 // icon shuffles, cycled round after round
const ROUNDS = 3;                   // rounds played on load; a click plays one more
const ZOOM = 2.8;                   // camera zoom when close on the icon
const CAM_EASE = 700;               // ms time constant of the camera following the growth

const CAPTIONS = [
  'Any agent ⇄ any provider',
  'Shared keys, rules and usage for your team',
  'Guardrails, MCP tools and remote control',
];

interface Walker { idx: number; prev: number; since: number; bornAt: number; dieAt: number }
interface Round { variant: number; cells: Map<number, number>; walkers: Walker[]; lastTick: number; ticks: number; doneAt: number }
/** Camera: focus point (grid units from the T's centre) and zoom. */
interface Camera { x: number; y: number; z: number }

function rng(seed: number): () => number {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = Math.imul(s ^ (s >>> 15), 1 | s);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
const clamp01 = (v: number): number => Math.max(0, Math.min(1, v));
const easeInOut = (t: number): number => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2);

function shuffled<T>(list: T[], r: () => number): T[] {
  const a = list.slice();
  for (let i = a.length - 1; i > 0; i--) {
    const j = (r() * (i + 1)) | 0;
    [a[i], a[j]] = [a[j], a[i]];
  }
  return a;
}

type Kind = 'agent' | 'channel' | 'provider';
interface Tile { icon: string; name: string; kind: Kind }

// Every distinct icon once (the OpenAI / Anthropic / DeepSeek logos appear as
// both agent-side SDKs and providers; the first claim wins).
const ALL_TILES: Tile[] = (() => {
  const seen = new Set<string>();
  const out: Tile[] = [];
  const add = (kind: Kind, list: typeof AGENTS): void => {
    for (const b of list) if (!seen.has(b.icon)) { seen.add(b.icon); out.push({ icon: b.icon, name: b.name, kind }); }
  };
  add('provider', PROVIDERS);
  add('agent', AGENTS);
  add('channel', CHANNELS);
  return out;
})();

// Tile per cell for each round. Cells prefer their side's kind (agents left,
// channels centre, providers right); a side that runs short borrows unused
// icons from the rest, so nothing repeats within a T until every icon is used.
// Each round reshuffles, so consecutive rounds show different mixes.
const layouts: Tile[][] = Array.from({ length: VARIANTS }, (_, v) => {
  const r = rng(1000 + v * 97);
  const unused = new Set(shuffled(ALL_TILES, r));
  const take = (kind: Kind): Tile => {
    if (!unused.size) ALL_TILES.forEach((t) => unused.add(t));   // every icon used: start over
    let pick: Tile | undefined;
    for (const t of unused) if (t.kind === kind) { pick = t; break; }
    pick ??= unused.values().next().value as Tile;
    unused.delete(pick);
    return pick;
  };
  const out: Tile[] = new Array(N * N);
  // fill in random cell order so borrowed icons do not always land in the same spots
  for (const i of shuffled(shapeCells.filter((c) => c !== START), r)) out[i] = take(side(i));
  return out;
});

// farthest cell from the start, in grid steps; orders the fold
const MAX_DIST = Math.max(...shapeCells.map((i) => Math.abs(col(i) - col(START)) + Math.abs(row(i) - row(START))));

export interface HeroOptions {
  /** receives the line describing the current round */
  caption?: HTMLElement | null;
  /** the hero copy; on wide screens the T is fitted into the space to its right */
  textColumn?: HTMLElement | null;
  /** positioned label shown when hovering a tile */
  tooltip?: HTMLElement | null;
}

export function startHero(canvas: HTMLCanvasElement, { caption, textColumn, tooltip }: HeroOptions = {}): void {
  const ctx = ctx2d(canvas);
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  const icons = new Map<string, HTMLCanvasElement>();
  let brand = makeCanvas(1);
  let palette: Palette | null = null;
  let W = 0, H = 0, originX = 0, originY = 0, boxSize = 0;
  let dotColor = '#b9bcc4';
  let accent = '#2563eb';

  let round: Round = newRound(0, 0);
  const random = rng(0xc0ffee);
  // `finished`: resting on the complete T with the loop stopped
  let running = false, finished = false, visible = false, raf = 0, lastFrame = 0;
  let roundsLeft = ROUNDS - 1;       // rounds still to start after the current one
  const startCam = (): Camera => ({ x: col(START) - CENTER, y: row(START) - CENTER, z: ZOOM });
  let cam: Camera = startCam();

  function newRound(variant: number, now: number): Round {
    return {
      variant,
      cells: new Map([[START, now - POP * 2]]),
      walkers: [{ idx: START, prev: START, since: now, bornAt: now, dieAt: 0 }],
      lastTick: now, ticks: 0, doneAt: 0,
    };
  }

  function step(r: Round, now: number): void {
    // after a background tab or a pause, pick up where we were instead of catching up
    if (now - r.lastTick > 1000) r.lastTick = now - TICK;
    while (!r.doneAt && now - r.lastTick >= TICK) {
      r.lastTick += TICK;
      r.ticks++;
      const t = r.lastTick;
      const taken = new Set(r.walkers.filter((w) => !w.dieAt).map((w) => w.idx));
      const kept: Walker[] = [];
      for (const w of r.walkers) {
        if (w.dieAt) { if (t - w.dieAt < FADE) kept.push(w); continue; }
        if (!r.cells.has(w.idx)) r.cells.set(w.idx, t);
        const options = (neighbours.get(w.idx) ?? []).filter((j) => !r.cells.has(j) && !taken.has(j));
        if (!options.length) { w.dieAt = t; kept.push(w); continue; }
        const next = options[(random() * options.length) | 0];
        w.prev = w.idx; w.idx = next; w.since = t;
        taken.add(next);
        kept.push(w);
      }
      r.walkers = kept;

      const alive = kept.filter((w) => !w.dieAt).length;
      const frontier: number[] = [];
      const seen = new Set<number>();
      for (const i of r.cells.keys()) {
        for (const j of neighbours.get(i) ?? []) {
          if (!r.cells.has(j) && !taken.has(j) && !seen.has(j)) { seen.add(j); frontier.push(j); }
        }
      }
      if (!frontier.length && !alive) { r.doneAt = t; break; }
      const target = Math.min(8, Math.max(1, Math.floor((r.ticks / 3) ** 2)));
      for (let n = alive; n < target && frontier.length; n++) {
        const k = (random() * frontier.length) | 0;
        const j = frontier[k];
        frontier[k] = frontier[frontier.length - 1];
        frontier.pop();
        r.walkers.push({ idx: j, prev: j, since: t, bornAt: t, dieAt: 0 });
      }
    }
  }

  function setCaption(i: number): void {
    if (caption && caption.dataset.round !== String(i)) {
      caption.dataset.round = String(i);
      caption.textContent = CAPTIONS[i % CAPTIONS.length];
    }
  }

  // ---------- camera ----------
  /** Where the camera wants to be: framing what has grown so far, with room to grow. */
  function camTarget(fold: number): Camera {
    if (finished) return { x: 0, y: 0, z: 1 };
    if (fold > 0) return startCam();
    let x0 = Infinity, x1 = -Infinity, y0 = Infinity, y1 = -Infinity;
    const grow = (i: number): void => {
      x0 = Math.min(x0, col(i)); x1 = Math.max(x1, col(i));
      y0 = Math.min(y0, row(i)); y1 = Math.max(y1, row(i));
    };
    for (const i of round.cells.keys()) grow(i);
    for (const w of round.walkers) grow(w.idx);
    const extent = Math.max(x1 - x0, y1 - y0) + TILE + (round.doneAt ? 0 : 1.2);
    const z = Math.max(1, Math.min(ZOOM, SPAN / extent));
    // at zoom 1 the whole T is framed, so centre on it; closer in, centre on the growth
    const k = (z - 1) / (ZOOM - 1);
    return { x: ((x0 + x1) / 2 - CENTER) * k, y: ((y0 + y1) / 2 - CENTER) * k, z };
  }

  function moveCam(dt: number, fold: number): void {
    const target = camTarget(fold);
    // during the fold push in a little faster, so the zoom lands with the last tile
    const f = 1 - Math.exp(-dt / (fold > 0 ? CAM_EASE * 0.6 : CAM_EASE));
    cam = { x: cam.x + (target.x - cam.x) * f, y: cam.y + (target.y - cam.y) * f, z: cam.z + (target.z - cam.z) * f };
  }

  /** Screen position of a grid coordinate under the current camera. */
  const sx = (c: number): number => originX + (c - CENTER - cam.x) * (boxSize / SPAN) * cam.z;
  const sy = (r: number): number => originY + (r - CENTER - cam.y) * (boxSize / SPAN) * cam.z;

  // ---------- drawing ----------
  function drawTile(img: CanvasImageSource, x: number, y: number, size: number, alpha: number): void {
    if (alpha <= 0.004 || size < 0.5) return;
    ctx.globalAlpha = alpha;
    ctx.drawImage(img, x - size / 2, y - size / 2, size, size);
  }

  /** Draws the grown cells of the T. */
  function drawT(r: Round, tile: number, now: number): void {
    for (const [i, bornAt] of r.cells) {
      const x = sx(col(i));
      const y = sy(row(i));
      let s = tile;
      let a = 1;
      const age = now - bornAt;
      if (age < POP) {
        const t = age / POP;
        s *= Math.max(0, 1 + 2.70158 * (t - 1) ** 3 + 1.70158 * (t - 1) ** 2);
        a *= Math.min(1, age / 120);
      }
      const img = i === START ? brand : icons.get(layouts[r.variant][i].icon);
      if (img) drawTile(img, x, y, s, a);
    }
    ctx.globalAlpha = 1;
  }

  function dots(spacing: number, ox: number, oy: number, alpha: number): void {
    const sx = ((ox % spacing) + spacing) % spacing;
    const sy = ((oy % spacing) + spacing) % spacing;
    ctx.fillStyle = dotColor;
    ctx.globalAlpha = alpha;
    for (let y = sy; y <= H; y += spacing) for (let x = sx; x <= W; x += spacing) ctx.fillRect(x - 1, y - 1, 2, 2);
    ctx.globalAlpha = 1;
  }

  function draw(now: number): void {
    ctx.clearRect(0, 0, W, H);
    const p = (boxSize / SPAN) * cam.z;
    const tile = p * TILE;
    dots(p, sx(0.5), sy(0.5), 0.5);

    const fold = foldAt(now);
    if (fold > 0) drawFold(fold, tile);
    else drawT(round, tile, now);

    // walkers: small "requests" hopping between cells, with a short trail
    const radius = Math.max(2, Math.min(7, tile * 0.075));
    ctx.fillStyle = accent;
    ctx.strokeStyle = accent;
    ctx.lineCap = 'round';
    ctx.lineWidth = radius * 1.2;
    for (const w of round.walkers) {
      const life = w.dieAt
        ? (1 - Math.min(1, (now - w.dieAt) / FADE)) ** 2
        : 1 - (1 - Math.min(1, (now - w.bornAt) / FADE)) ** 3;
      if (life <= 0) continue;
      const e = 1 - (1 - Math.min(1, (now - w.since) / (TICK * 0.9))) ** 2;
      const ax = sx(col(w.prev)), ay = sy(row(w.prev));
      const bx = sx(col(w.idx)), by = sy(row(w.idx));
      const x = ax + (bx - ax) * e, y = ay + (by - ay) * e;
      if (e < 1) {
        ctx.globalAlpha = 0.3 * life * (1 - e);
        ctx.beginPath(); ctx.moveTo(ax, ay); ctx.lineTo(x, y); ctx.stroke();
      }
      ctx.globalAlpha = 0.14 * life;
      ctx.beginPath(); ctx.arc(x, y, radius * 2.2, 0, Math.PI * 2); ctx.fill();
      ctx.globalAlpha = life;
      ctx.beginPath(); ctx.arc(x, y, radius, 0, Math.PI * 2); ctx.fill();
    }
    ctx.globalAlpha = 1;
  }

  /**
   * The finished T drains into the Tingly Box icon: tiles fly to the start cell
   * nearest-first and vanish into it, the icon swells as it takes them in, and
   * a ring goes out as the last one lands.
   */
  function drawFold(t: number, tile: number): void {
    const bx = sx(col(START));
    const by = sy(row(START));
    for (const [i] of round.cells) {
      if (i === START) continue;
      const d = Math.abs(col(i) - col(START)) + Math.abs(row(i) - row(START));
      const local = clamp01((t - (d / MAX_DIST) * 0.5) / 0.42);
      const e = local * local;
      const x = sx(col(i));
      const y = sy(row(i));
      const img = icons.get(layouts[round.variant][i].icon);
      if (img) drawTile(img, x + (bx - x) * e, y + (by - y) * e, tile * (1 - 0.7 * e), 1 - clamp01((e - 0.55) / 0.45));
    }
    const ring = clamp01((t - 0.62) / 0.38);
    if (ring > 0 && ring < 1) {
      const r = tile * (1 + 1.1 * easeInOut(ring));
      ctx.globalAlpha = 0.45 * (1 - ring);
      ctx.strokeStyle = accent;
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.roundRect(bx - r / 2, by - r / 2, r, r, r * 0.3);
      ctx.stroke();
    }
    const swell = 1 + 0.12 * Math.sin(Math.PI * clamp01((t - 0.1) / 0.9));
    drawTile(brand, bx, by, tile * swell, 1);
    ctx.globalAlpha = 1;
  }

  // ---------- hover labels ----------
  function brandAt(i: number): { name: string; kind: string } | null {
    if (i === START) return { name: 'Tingly Box', kind: 'gateway' };
    const t = layouts[round.variant][i];
    return t ? { name: t.name, kind: t.kind === 'channel' ? 'remote channel' : t.kind } : null;
  }

  function mountTooltip(tip: HTMLElement): void {
    if (!window.matchMedia('(hover: hover)').matches) return;
    const hide = (): void => { tip.hidden = true; };
    canvas.addEventListener('pointerleave', hide);
    canvas.addEventListener('pointermove', (ev) => {
      const rect = canvas.getBoundingClientRect();
      const p = (boxSize / SPAN) * cam.z;
      const c = Math.round((ev.clientX - rect.left - originX) / p + CENTER + cam.x);
      const r = Math.round((ev.clientY - rect.top - originY) / p + CENTER + cam.y);
      const i = r * N + c;
      const folding = foldAt(performance.now()) > 0;
      const info = c >= 0 && r >= 0 && c < N && r < N && round.cells.has(i) && !folding ? brandAt(i) : null;
      if (!info) { hide(); return; }
      tip.innerHTML = '';
      const kind = document.createElement('small');
      kind.textContent = info.kind;
      tip.append(kind, info.name);
      tip.style.left = `${canvas.offsetLeft + sx(c)}px`;
      tip.style.top = `${canvas.offsetTop + sy(r) - (p * TILE) / 2}px`;
      tip.hidden = false;
    });
  }

  // ---------- loop ----------
  /** Fold progress 0..1 of the current round (0 while growing, holding or resting). */
  function foldAt(now: number): number {
    return round.doneAt && !finished ? clamp01((now - round.doneAt - HOLD) / FOLD) : 0;
  }

  function frame(now: number): void {
    if (!running) return;
    const dt = Math.min(100, now - (lastFrame || now));
    lastFrame = now;
    step(round, now);
    if (round.doneAt && !roundsLeft && now - round.doneAt > HOLD) {
      // out of rounds: rest on the finished T and stop drawing until woken
      rest();
      return;
    }
    if (round.doneAt && now - round.doneAt > HOLD + FOLD) {
      roundsLeft--;
      round = newRound((round.variant + 1) % VARIANTS, now);
      setCaption(round.variant);
    }
    moveCam(dt, foldAt(now));
    draw(now);
    raf = requestAnimationFrame(frame);
  }

  function pause(): void {
    running = false;
    cancelAnimationFrame(raf);
  }

  function resume(): void {
    if (finished || !visible) return;
    cancelAnimationFrame(raf);
    running = true;
    lastFrame = 0;
    raf = requestAnimationFrame(frame);
  }

  function setIdle(idle: boolean): void {
    canvas.classList.toggle('is-idle', idle);
    caption?.closest('.hero-level')?.classList.toggle('is-idle', idle);
  }

  function rest(): void {
    pause();
    finished = true;
    round.walkers = [];
    cam = { x: 0, y: 0, z: 1 };
    setIdle(true);
    draw(performance.now() + 1e6);
  }

  /** Clicking the resting T folds it and plays one more round. */
  function wake(): void {
    if (!finished || reducedMotion) return;
    finished = false;
    roundsLeft = 1;
    round.doneAt = performance.now() - HOLD;   // start folding right away
    if (tooltip) tooltip.hidden = true;
    setIdle(false);
    resume();
  }

  /** The finished T, without animation (reduced motion). */
  function showFinal(): void {
    round = newRound(0, -1e6);
    for (const i of shapeCells) round.cells.set(i, -1e6);
    round.walkers = [];
    round.doneAt = -1e6;
    finished = true;
    cam = { x: 0, y: 0, z: 1 };
    setCaption(0);
    draw(0);
  }

  function readPalette(): Palette {
    const css = getComputedStyle(canvas);
    const read = (name: string, fallback: string): string => css.getPropertyValue(name).trim() || fallback;
    dotColor = read('--hero-dot', '#b9bcc4');
    accent = read('--accent', '#2563eb');
    return { brandBg: read('--hero-brand-bg', '#1d1f24'), brandFg: read('--hero-brand-fg', '#ffffff') };
  }

  function layout(): void {
    const dpr = Math.min(2, window.devicePixelRatio || 1);
    W = canvas.clientWidth;
    H = canvas.clientHeight;
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingQuality = 'high';
    const wide = W >= 960;
    if (wide) {
      // fit between the text column and the right edge, with breathing room
      let textRight = W * 0.45;
      if (textColumn) {
        const left = canvas.getBoundingClientRect().left;
        textRight = Math.max(...[...textColumn.children].map((c) => c.getBoundingClientRect().right)) - left;
      }
      const from = textRight + 64;
      const to = W - Math.max(48, (W - 1200) / 2);
      originX = (from + to) / 2;
      boxSize = Math.max(160, Math.min(to - from, H * 0.78));
    } else {
      originX = W * 0.5;
      boxSize = Math.min(W * 0.9, H * 0.9);
    }
    originY = H * 0.5;
    const next = readPalette();
    if (!palette || next.brandBg !== palette.brandBg || next.brandFg !== palette.brandFg) {
      palette = next;
      brand = tTile(next.brandBg, next.brandFg);
    }
    if (!running) draw(performance.now() + (finished ? 1e6 : 0));
  }

  // ---------- boot ----------
  const urls = ALL_TILES.map((t) => t.icon);
  const fontsReady = document.fonts ? document.fonts.ready : Promise.resolve();
  Promise.all([fontsReady, ...urls.map(async (u) => icons.set(u, iconTile(await loadSvg(u))))]).then(() => {
    layout();
    new ResizeObserver(layout).observe(canvas);
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', layout);

    if (tooltip) mountTooltip(tooltip);
    if (reducedMotion) { showFinal(); return; }
    round = newRound(0, performance.now());
    cam = startCam();
    setCaption(0);
    canvas.addEventListener('click', wake);
    // loop while visible, pause (keeping the current frame) while scrolled away
    new IntersectionObserver(([entry]) => {
      visible = entry.isIntersecting;
      if (visible) resume();
      else pause();
    }, { threshold: 0.15 }).observe(canvas);
  });
}
