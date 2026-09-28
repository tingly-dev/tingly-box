// "How it works": a sticky SVG stage driven by scroll. Each step reveals its
// pieces (cards pop in, edges draw along their length) while the camera — the
// SVG viewBox — pans from the agents out to the whole team picture.
// Narrow screens and reduced motion get the complete picture, statically.
import { byName, AGENTS, CHANNELS, PROVIDERS } from '../data/brands';
import { iconSvg } from '../ui/icons';

type Item =
  | { kind: 'edge'; el: SVGGElement; step: number; order: number }
  | { kind: 'pop'; el: SVGGElement; step: number; order: number; origin: [number, number] };

const NS = 'http://www.w3.org/2000/svg';
const STEPS = 5;
const STEP_GAP = 0.9;
const STEP_ANIM = 0.7;
// Scroll progress 0..1 maps to animation time T0..T0+T_SPAN.
const T0 = 0.5;
const T_SPAN = (STEPS - 1) * STEP_GAP + STEP_ANIM - 0.3;

// camera stop per step: x, y, w, h in viewBox units
const CAMERA: number[][] = [
  [-20, 140, 560, 358],
  [30, 120, 660, 422],
  [40, 110, 920, 520],
  [30, 30, 940, 560],
  [0, 20, 1000, 620],
];
const FULL = [0, 20, 1000, 620];

const clamp01 = (v: number): number => Math.max(0, Math.min(1, v));
const ease = (t: number): number => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2);
const backOut = (t: number): number => 1 + 2.2 * (t - 1) ** 3 + 1.2 * (t - 1) ** 2;

export function mountFlow(svg: SVGSVGElement, scroller: HTMLElement): void {
  const items: Item[] = [];

  function el<K extends keyof SVGElementTagNameMap>(tag: K, attrs: Record<string, string | number>, parent: Element = svg): SVGElementTagNameMap[K] {
    const n = document.createElementNS(NS, tag);
    for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, String(v));
    parent.append(n);
    return n;
  }
  function text(parent: Element, x: number, y: number, cls: string, value: string, attrs: Record<string, string | number> = {}): void {
    el('text', { x, y, class: cls, ...attrs }, parent).textContent = value;
  }
  function pop(step: number, order: number, origin: [number, number]): SVGGElement {
    const g = el('g', {});
    items.push({ kind: 'pop', el: g, step, order, origin });
    return g;
  }
  function edge(step: number, order: number, d: string): void {
    const g = el('g', {});
    el('path', { d, class: 'fl-edge', pathLength: 1 }, g);
    el('path', { d, class: 'fl-flow', pathLength: 1 }, g);
    items.push({ kind: 'edge', el: g, step, order });
  }
  function iconCard(parent: Element, x: number, y: number, icon: string, size = 40): void {
    el('rect', { x: x - size / 2, y: y - size / 2, width: size, height: size, rx: size * 0.26, fill: '#fff', stroke: 'rgba(22,24,29,0.14)' }, parent);
    el('image', { href: icon, x: x - size * 0.29, y: y - size * 0.29, width: size * 0.58, height: size * 0.58 }, parent);
  }
  /** A white icon card like iconCard, holding a Tabler icon instead of a brand logo. */
  function tablerCard(parent: Element, x: number, y: number, name: string, size = 24): void {
    el('rect', { x: x - size / 2, y: y - size / 2, width: size, height: size, rx: size * 0.26, fill: '#fff', stroke: 'rgba(22,24,29,0.14)' }, parent);
    const icon = new DOMParser().parseFromString(iconSvg(name), 'image/svg+xml').documentElement;
    const s = size * 0.62;
    for (const [k, v] of Object.entries({ x: x - s / 2, y: y - s / 2, width: s, height: s, class: 'fl-tabler' })) icon.setAttribute(k, String(v));
    parent.append(document.importNode(icon, true));
  }
  /** Lays out four icons centred under a bottom-row card title. */
  function iconRow(cx: number, draw: (x: number, i: number) => void): void {
    for (let i = 0; i < 4; i++) draw(cx - 49 + i * 30, i);
  }
  function card(step: number, order: number, cx: number, title: string): SVGGElement {
    const g = pop(step, order, [cx, 575]);
    el('rect', { x: cx - 75, y: 544, width: 150, height: 62, rx: 12, class: 'fl-card' }, g);
    text(g, cx - 61, 566, 'fl-title', title);
    return g;
  }

  const BOX = { x: 500, y: 330, s: 136 };
  const agents = ['Claude Code', 'Codex', 'OpenCode', 'Cursor', 'Xcode'].map((n) => byName(AGENTS, n));
  const providers = ['Anthropic', 'OpenAI', 'Gemini', 'DeepSeek', 'Qwen', 'Ollama'].map((n) => byName(PROVIDERS, n));
  const members = ['AL', 'BK', 'CS', 'DM'];
  const bottom = [260, 420, 580, 740];
  const agentY = (i: number): number => 206 + i * 62;
  const provY = (i: number): number => 188 + i * 56;
  const memberX = (i: number): number => 380 + i * 80;

  // edges first so cards sit on top of them
  agents.forEach((_, i) => edge(1, i, `M 236 ${agentY(i)} C 330 ${agentY(i)}, 340 ${BOX.y}, ${BOX.x - BOX.s / 2 - 6} ${BOX.y}`));
  providers.forEach((_, i) => edge(2, i, `M ${BOX.x + BOX.s / 2 + 6} ${BOX.y} C 670 ${BOX.y}, 690 ${provY(i)}, 776 ${provY(i)}`));
  members.forEach((_, i) => edge(3, i, `M ${memberX(i)} 126 C ${memberX(i)} 170, 500 160, 500 ${BOX.y - BOX.s / 2 - 40}`));
  bottom.forEach((x, i) => edge(4, i, `M 500 ${BOX.y + BOX.s / 2 + 64} C 500 520, ${x} 510, ${x} 540`));

  // 01 — agents
  agents.forEach((a, i) => {
    const g = pop(0, i, [110, agentY(i)]);
    iconCard(g, 110, agentY(i), a.icon);
    text(g, 142, agentY(i) + 5, 'fl-label', a.name);
  });

  // 02 — the box and its endpoint
  {
    const g = pop(1, 0, [BOX.x, BOX.y]);
    el('rect', { x: BOX.x - BOX.s / 2, y: BOX.y - BOX.s / 2, width: BOX.s, height: BOX.s, rx: BOX.s * 0.28, class: 'fl-brand' }, g);
    text(g, BOX.x, BOX.y + 24, 'fl-brand-t', 'T', { 'text-anchor': 'middle' });
    const e = pop(1, 2, [BOX.x, BOX.y - BOX.s / 2 - 27]);
    el('rect', { x: BOX.x - 118, y: BOX.y - BOX.s / 2 - 40, width: 236, height: 26, rx: 13, class: 'fl-accent-soft' }, e);
    text(e, BOX.x, BOX.y - BOX.s / 2 - 23, 'fl-accent-text', 'localhost:12580/tingly/…', { 'text-anchor': 'middle' });
  }

  // 03 — providers and a routing rule
  providers.forEach((p, i) => {
    const g = pop(2, i + 1, [800, provY(i)]);
    iconCard(g, 800, provY(i), p.icon, 36);
    text(g, 828, provY(i) + 5, 'fl-label', p.name);
  });
  {
    // a routing rule, drawn as a route: model name → primary provider ┄→ fallback
    const y = BOX.y + BOX.s / 2 + 38;
    const g = pop(2, 0, [BOX.x, y]);
    el('rect', { x: BOX.x - 150, y: y - 24, width: 300, height: 48, rx: 12, class: 'fl-card' }, g);
    el('rect', { x: BOX.x - 136, y: y - 12, width: 118, height: 24, rx: 12, class: 'fl-accent-soft' }, g);
    text(g, BOX.x - 77, y + 4, 'fl-accent-text', 'claude-sonnet', { 'text-anchor': 'middle' });
    const arrow = (x1: number, x2: number, dashed: boolean): void => {
      el('path', { d: `M ${x1} ${y} H ${x2 - 5}`, class: dashed ? 'fl-rule fl-rule-dashed' : 'fl-rule' }, g);
      el('path', { d: `M ${x2 - 6} ${y - 4} L ${x2} ${y} L ${x2 - 6} ${y + 4}`, class: 'fl-rule' }, g);
    };
    arrow(BOX.x - 12, BOX.x + 22, false);
    iconCard(g, BOX.x + 42, y, byName(PROVIDERS, 'Anthropic').icon, 30);
    arrow(BOX.x + 62, BOX.x + 96, true);
    const fallback = el('g', { opacity: 0.55 }, g);
    iconCard(fallback, BOX.x + 116, y, byName(PROVIDERS, 'DeepSeek').icon, 30);
  }

  // 04 — team members, one sharing key each
  members.forEach((initials, i) => {
    const x = memberX(i);
    const g = pop(3, i, [x, 96]);
    el('circle', { cx: x, cy: 88, r: 22, class: 'fl-soft' }, g);
    text(g, x, 93, 'fl-title', initials, { 'text-anchor': 'middle' });
    el('rect', { x: x - 30, y: 114, width: 60, height: 18, rx: 9, class: 'fl-card' }, g);
    text(g, x, 127, 'fl-mono fl-small', `sk-••${i + 3}f`, { 'text-anchor': 'middle' });
  });
  {
    const g = pop(3, 5, [220, 88]);
    text(g, 300, 80, 'fl-title', 'Your team', { 'text-anchor': 'end' });
    text(g, 300, 98, 'fl-mono', 'one key each', { 'text-anchor': 'end' });
  }

  // 05 — govern, extend & observe: every card is a row of icons
  const tablerRow = (step: number, order: number, title: string, names: string[]): void => {
    const g = card(step, order, bottom[order], title);
    iconRow(bottom[order], (x, i) => tablerCard(g, x, 588, names[i]));
  };
  tablerRow(4, 0, 'Guardrails', ['shield-check', 'eye-off', 'lock', 'users']);
  tablerRow(4, 1, 'MCP tools', ['world-search', 'world-www', 'folder', 'plus']);
  tablerRow(4, 2, 'Usage', ['chart-bar', 'coin', 'clock', 'users']);
  {
    const g = card(4, 3, bottom[3], 'Remote');
    const channels = ['Telegram', 'Weixin', 'Feishu / Lark', 'DingTalk'].map((n) => byName(CHANNELS, n).icon);
    iconRow(bottom[3], (x, i) => iconCard(g, x, 588, channels[i], 24));
  }

  const steps = [...document.querySelectorAll<HTMLElement>('.flow-step')];

  function render(t: number, dynamic: boolean): void {
    const reveal = Array.from({ length: STEPS }, (_, k) => (dynamic ? clamp01((t - k * STEP_GAP) / STEP_ANIM) : 1));
    for (const it of items) {
      const local = dynamic ? clamp01((reveal[it.step] * STEP_ANIM - it.order * 0.05) / (STEP_ANIM * 0.55)) : 1;
      if (it.kind === 'edge') {
        it.el.style.opacity = local > 0 ? '1' : '0';
        (it.el.firstChild as SVGPathElement).style.strokeDashoffset = String(1 - local);
        (it.el.lastChild as SVGPathElement).style.opacity = local >= 1 ? '1' : '0';
      } else {
        const s = local <= 0 ? 0 : backOut(local);
        const [ox, oy] = it.origin;
        it.el.style.opacity = String(Math.min(1, local * 2));
        it.el.setAttribute('transform', `translate(${ox} ${oy}) scale(${s}) translate(${-ox} ${-oy})`);
      }
    }

    let box = FULL;
    if (dynamic) {
      const c = reveal.slice(1).reduce((a, v) => a + ease(v), 0);
      const i = Math.min(CAMERA.length - 2, Math.floor(c));
      const f = c - i;
      box = CAMERA[i].map((v, j) => v + (CAMERA[i + 1][j] - v) * f);
    }
    svg.setAttribute('viewBox', box.map((v) => v.toFixed(2)).join(' '));

    let active = 0;
    for (let k = 0; k < STEPS; k++) if (t >= k * STEP_GAP) active = k;
    steps.forEach((s, k) => {
      s.dataset.state = !dynamic ? 'static' : k === active ? 'active' : k < active ? 'done' : 'upcoming';
      if (dynamic && k === active) s.setAttribute('aria-current', 'step');
      else s.removeAttribute('aria-current');
    });
  }

  const wide = window.matchMedia('(min-width: 992px)');
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)');
  let dynamic = false, near = false, target = 0, current = 0, raf = 0;

  function measure(): void {
    const rect = scroller.getBoundingClientRect();
    const span = rect.height - window.innerHeight;
    const p = span > 0 ? clamp01(-rect.top / span) : 1;
    target = T0 + p * T_SPAN;
  }
  // Click a step to jump to it: scroll to the position whose progress lands
  // just after that step finishes animating, so scroll and click stay in sync.
  function goTo(k: number): void {
    if (!dynamic) return;
    const t = Math.min(T0 + T_SPAN, k * STEP_GAP + STEP_ANIM);
    const span = scroller.offsetHeight - window.innerHeight;
    const top = scroller.getBoundingClientRect().top + window.scrollY;
    window.scrollTo({ top: top + clamp01((t - T0) / T_SPAN) * span, behavior: 'smooth' });
  }
  steps.forEach((s, k) => {
    s.addEventListener('click', () => goTo(k));
    s.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); goTo(k); }
    });
  });
  function loop(): void {
    raf = 0;
    const d = target - current;
    current = Math.abs(d) < 0.0005 ? target : current + d * 0.14;
    render(current, true);
    if (current !== target && near) raf = requestAnimationFrame(loop);
  }
  function onScroll(): void {
    if (!dynamic) return;
    measure();
    if (!raf) raf = requestAnimationFrame(loop);
  }
  function setMode(): void {
    dynamic = wide.matches && !reduced.matches;
    scroller.dataset.static = String(!dynamic);
    steps.forEach((s) => {
      if (dynamic) { s.tabIndex = 0; s.setAttribute('role', 'button'); }
      else { s.removeAttribute('tabindex'); s.removeAttribute('role'); }
    });
    if (dynamic) { measure(); current = target; render(current, true); } else render(0, false);
  }

  new IntersectionObserver(([e]) => { near = e.isIntersecting; onScroll(); }, { rootMargin: '200px 0px' }).observe(scroller);
  window.addEventListener('scroll', onScroll, { passive: true });
  window.addEventListener('resize', onScroll);
  wide.addEventListener('change', setMode);
  reduced.addEventListener('change', setMode);
  setMode();
}
