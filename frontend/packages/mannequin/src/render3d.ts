// The manikin, drawn as a figure-drawing line sheet: white, unshaded forms,
// a bold ink silhouette, finer ink wherever one form turns away or passes in
// front of another, and light construction lines (the joint seams and the
// Loomis head lines) on the near side. The camera is the one `projectionOf`
// describes, to the pixel, so the blue handles drawn on top from
// `projectFigure` sit exactly on the joints they move.
//
// Everything is drawn into one shared WebGL canvas and copied onto the caller's
// 2D context: the sketch surface stays a plain canvas (strokes, export and the
// mock backend never learn that WebGL exists), and one context serves every
// thumbnail in the pose library — browsers cap live WebGL contexts at about a
// dozen, and the library alone has forty-four tiles.
import {
    BufferAttribute,
    BufferGeometry,
    Color,
    DepthTexture,
    Mesh,
    MeshBasicMaterial,
    MeshNormalMaterial,
    NearestFilter,
    OrthographicCamera,
    PerspectiveCamera,
    PlaneGeometry,
    Scene,
    ShaderMaterial,
    Vector2,
    WebGLRenderTarget,
    WebGLRenderer,
} from 'three';
import { LineMaterial } from 'three/examples/jsm/lines/LineMaterial.js';
import { LineSegments2 } from 'three/examples/jsm/lines/LineSegments2.js';
import { LineSegmentsGeometry } from 'three/examples/jsm/lines/LineSegmentsGeometry.js';
import { figureLineEdges, figureSurface, toneFor } from './body';
import { projectionOf, projectPoint } from './camera';
import { figureUnit, type PoseFigure } from './skeleton';

const INK = '#26292e';
const inkColour = new Color(INK);
const CONSTRUCTION = '#9aa0a8';

// The fill: flat, the paper's own white (or the figure's tone). A line sheet
// carries form by its lines, and shading would only muddy what the image
// model reads first.
const fills = new Map<string, MeshBasicMaterial>();
const fillFor = (hex: string): MeshBasicMaterial => {
    let material = fills.get(hex);
    if (!material) {
        material = new MeshBasicMaterial({ color: hex });
        fills.set(hex, material);
    }
    return material;
};

// The construction lines: thin, light, and only where they are on the near
// side of the body — the reference sheets never show a seam through a form.
const lineMaterials = new Map<number, LineMaterial>();
const lineMaterialFor = (width: number, size: Vector2): LineMaterial => {
    const key = Math.round(width * 10) / 10;
    let material = lineMaterials.get(key);
    if (!material) {
        material = new LineMaterial({ color: CONSTRUCTION, linewidth: key, transparent: true, opacity: 0.9 });
        lineMaterials.set(key, material);
    }
    material.resolution.copy(size);
    return material;
};

const constructionLines = (figure: PoseFigure, geometry: BufferGeometry, size: Vector2, sample: number): { mesh: LineSegments2; geometry: LineSegmentsGeometry } => {
    const { a, b, t } = figureLineEdges(figure);
    const position = geometry.getAttribute('position');
    const normal = geometry.getAttribute('normal');
    const lift = figureUnit(figure) * 0.003;
    const points = new Float32Array(a.length * 3);
    for (let k = 0; k < a.length; k += 1) {
        const f = t[k] / 255;
        for (let c = 0; c < 3; c += 1) {
            const p = position.array[a[k] * 3 + c] * (1 - f) + position.array[b[k] * 3 + c] * f;
            const n = normal.array[a[k] * 3 + c] * (1 - f) + normal.array[b[k] * 3 + c] * f;
            points[k * 3 + c] = p + n * lift;
        }
    }
    const lines = new LineSegmentsGeometry();
    lines.setPositions(points);
    const width = Math.max(0.8, Math.min(1.6, figureUnit(figure) * 0.0019)) * sample;
    return { mesh: new LineSegments2(lines, lineMaterialFor(width, size)), geometry: lines };
};

const surfaceGeometry = (figure: PoseFigure): BufferGeometry => {
    const { positions, index } = figureSurface(figure);
    // Into three's y-up space: one sign, as everywhere else.
    const flipped = new Float32Array(positions.length);
    for (let i = 0; i < positions.length; i += 3) {
        flipped[i] = positions[i];
        flipped[i + 1] = -positions[i + 1];
        flipped[i + 2] = positions[i + 2];
    }
    const geometry = new BufferGeometry();
    geometry.setAttribute('position', new BufferAttribute(flipped, 3));
    geometry.setIndex(new BufferAttribute(index, 1));
    geometry.computeVertexNormals();
    return geometry;
};

// --- the ink -----------------------------------------------------------------
//
// Lines come from the picture, not the mesh: the body is drawn once into a
// buffer of view-space normals and depth, and a full-screen pass inks every
// pixel where that buffer jumps. Three jumps, three kinds of line:
//
// - coverage (body / paper) — the silhouette, sampled wider, so it is bold;
// - depth — one form in front of another (an arm across the chest, a thigh
//   over a calf), the overlap lines a figure drawing is made of;
// - normal — a surface turning sharply away (the fold of a bent knee, the
//   underside of the bust and the glutes), the finest line.
//
// Line weight is set in pixels from the figure's size, so a thumbnail and a
// full canvas read as the same drawing.
const INK_SHADER = {
    vertexShader: /* glsl */ `
        varying vec2 vUv;
        void main() { vUv = uv; gl_Position = vec4(position.xy, 0.0, 1.0); }
    `,
    fragmentShader: /* glsl */ `
        uniform sampler2D tNormal;
        uniform sampler2D tDepth;
        uniform vec2 texel;
        uniform float near;
        uniform float far;
        uniform float inner;
        uniform float outer;
        uniform vec3 ink;
        varying vec2 vUv;

        float linear(float d) { return (near * far) / (far - d * (far - near)); }

        void main() {
            vec4 centre = texture2D(tNormal, vUv);
            float zc = linear(texture2D(tDepth, vUv).x);
            vec3 nc = centre.xyz * 2.0 - 1.0;
            float line = 0.0;
            // Silhouette, weighted like an inked drawing: heavy on the side
            // turned away from the light (below and to the right), light on
            // the lit side. Each direction is sampled at several radii; a
            // radius counts only up to the weight the body's normal there
            // asks for, so the stroke swells and thins round the form.
            vec3 light = normalize(vec3(-0.45, 0.75, 0.5));
            for (int i = 0; i < 12; i++) {
                float a = float(i) * 0.523599;
                vec2 dir = vec2(cos(a), sin(a));
                for (int k = 1; k <= 4; k++) {
                    float r = outer * float(k) * 0.35;
                    vec4 s = texture2D(tNormal, vUv + dir * r * texel);
                    if (abs(s.a - centre.a) < 0.5) continue;
                    vec3 n = (centre.a > 0.5 ? centre.xyz : s.xyz) * 2.0 - 1.0;
                    float shade = 1.0 - max(dot(n, light), 0.0);
                    float reach = outer * mix(0.62, 1.4, shade);
                    line = max(line, 1.0 - smoothstep(reach - 0.6, reach + 0.6, r));
                }
            }
            if (centre.a > 0.5) {
                for (int i = 0; i < 4; i++) {
                    float a = float(i) * 1.570796;
                    vec2 o = vec2(cos(a), sin(a)) * inner * texel;
                    vec4 s = texture2D(tNormal, vUv + o);
                    if (s.a < 0.5) continue;
                    float zs = linear(texture2D(tDepth, vUv + o).x);
                    // Depth: relative, so the threshold means the same near and far.
                    float dz = abs(zs - zc) / zc;
                    line = max(line, smoothstep(0.004, 0.014, dz));
                    // Normal: a crease or a turn away.
                    vec3 ns = s.xyz * 2.0 - 1.0;
                    line = max(line, smoothstep(0.12, 0.32, 1.0 - dot(nc, ns)) * 0.8);
                }
            }
            if (line < 0.02) discard;
            gl_FragColor = vec4(ink, line);
        }
    `,
};

const inkMaterial = new ShaderMaterial({
    uniforms: {
        tNormal: { value: null },
        tDepth: { value: null },
        texel: { value: new Vector2() },
        near: { value: 1 },
        far: { value: 10 },
        inner: { value: 1 },
        outer: { value: 1.5 },
        ink: { value: null },
    },
    vertexShader: INK_SHADER.vertexShader,
    fragmentShader: INK_SHADER.fragmentShader,
    transparent: true,
    depthTest: false,
    depthWrite: false,
});
const inkScene = new Scene();
const inkQuad = new Mesh(new PlaneGeometry(2, 2), inkMaterial);
inkScene.add(inkQuad);
const inkCamera = new OrthographicCamera(-1, 1, 1, -1, 0, 1);
const normals = new MeshNormalMaterial();

let target: WebGLRenderTarget | null = null;
const targetFor = (width: number, height: number): WebGLRenderTarget => {
    if (!target || target.width !== width || target.height !== height) {
        target?.dispose();
        target = new WebGLRenderTarget(width, height, { minFilter: NearestFilter, magFilter: NearestFilter });
        target.depthTexture = new DepthTexture(width, height);
    }
    return target;
};

// The camera `projectionOf` describes: a pinhole at the figure's anchor, one
// focal length in front of it, with the canvas as an off-centre window onto
// that view. A virtual frame big enough to contain the canvas on every side of
// the anchor is what lets the window be offset without moving the pinhole.
const cameraFor = (figure: PoseFigure, width: number, height: number): PerspectiveCamera => {
    const { anchor, distance } = projectionOf(figure);
    const fullWidth = 2 * Math.max(anchor.x, width - anchor.x) + 2;
    const fullHeight = 2 * Math.max(anchor.y, height - anchor.y) + 2;
    const fov = 2 * Math.atan((fullHeight / 2) / distance) * (180 / Math.PI);
    // Near sits where `perspectiveAt` starts clamping, so a joint dragged into
    // the lens clips instead of exploding across the canvas.
    const camera = new PerspectiveCamera(fov, fullWidth / fullHeight, distance * 0.42, distance * 6);
    camera.position.set(anchor.x, -anchor.y, (anchor.z ?? 0) + distance);
    camera.lookAt(anchor.x, -anchor.y, anchor.z ?? 0);
    camera.setViewOffset(fullWidth, fullHeight, fullWidth / 2 - anchor.x, fullHeight / 2 - anchor.y, width, height);
    camera.updateProjectionMatrix();
    return camera;
};

let shared: WebGLRenderer | null | undefined;
const rendererFor = (width: number, height: number): WebGLRenderer | null => {
    if (shared === undefined) {
        try {
            shared = new WebGLRenderer({ alpha: true, antialias: true, preserveDrawingBuffer: false });
            shared.setPixelRatio(1);
            shared.setClearColor(0x000000, 0);
        } catch {
            shared = null;
        }
    }
    if (!shared) return null;
    const size = shared.getSize(new Vector2());
    if (size.x !== width || size.y !== height) shared.setSize(width, height, false);
    return shared;
};

// What WebGL cannot draw, 2D draws flat: the same solids, projected, as
// silhouettes. Ugly, honest, and only ever seen where there is no GPU.
const drawFlat = (ctx: CanvasRenderingContext2D, figure: PoseFigure, hex: string): void => {
    const projection = projectionOf(figure);
    const { positions, index } = figureSurface(figure);
    const xs = new Float32Array(positions.length / 3);
    const ys = new Float32Array(positions.length / 3);
    for (let i = 0; i < xs.length; i += 1) {
        const p = projectPoint({ x: positions[i * 3], y: positions[i * 3 + 1], z: positions[i * 3 + 2] }, projection);
        xs[i] = p.x; ys[i] = p.y;
    }
    ctx.save();
    ctx.fillStyle = hex;
    ctx.beginPath();
    for (let t = 0; t < index.length; t += 3) {
        const a = index[t], b = index[t + 1], c = index[t + 2];
        ctx.moveTo(xs[a], ys[a]);
        ctx.lineTo(xs[b], ys[b]);
        ctx.lineTo(xs[c], ys[c]);
        ctx.closePath();
    }
    ctx.fill('nonzero');
    ctx.restore();
};

export const drawFigure = (
    ctx: CanvasRenderingContext2D,
    figure: PoseFigure,
    options: { selected?: boolean } = {},
): void => {
    const hex = toneFor(figure, options.selected === true).body;
    // Drawn at twice the size and copied down: the ink is found per pixel,
    // and without the supersample every line has stair-steps.
    const sample = Math.max(ctx.canvas.width, ctx.canvas.height) <= 1400 ? 2 : 1;
    const width = ctx.canvas.width * sample;
    const height = ctx.canvas.height * sample;
    const renderer = rendererFor(width, height);
    if (!renderer) {
        drawFlat(ctx, figure, hex);
        return;
    }
    const camera = cameraFor(figure, ctx.canvas.width, ctx.canvas.height);
    const geometry = surfaceGeometry(figure);
    const size = new Vector2(width, height);

    // 1. Normals and depth, off screen.
    const buffer = targetFor(width, height);
    const normalScene = new Scene();
    normalScene.add(new Mesh(geometry, normals));
    renderer.setRenderTarget(buffer);
    renderer.setClearColor(0x000000, 0);
    renderer.clear();
    renderer.render(normalScene, camera);
    renderer.setRenderTarget(null);

    // 2. The white body and its construction lines, depth-tested together.
    const scene = new Scene();
    const lines = constructionLines(figure, geometry, size, sample);
    const body = new Mesh(geometry, fillFor(hex));
    body.renderOrder = 0;
    lines.mesh.renderOrder = 1;
    scene.add(body, lines.mesh);
    renderer.render(scene, camera);

    // 3. The ink, over both.
    const u = figureUnit(figure) * sample;
    inkMaterial.uniforms.tNormal.value = buffer.texture;
    inkMaterial.uniforms.tDepth.value = buffer.depthTexture;
    inkMaterial.uniforms.texel.value.set(1 / width, 1 / height);
    inkMaterial.uniforms.near.value = camera.near;
    inkMaterial.uniforms.far.value = camera.far;
    inkMaterial.uniforms.inner.value = Math.min(2.2 * sample, Math.max(1, u * 0.0026));
    // Capped: a pen has a width, and a big figure should not get a brush.
    inkMaterial.uniforms.outer.value = Math.min(3.2 * sample, Math.max(1.6, u * 0.0052));
    inkMaterial.uniforms.ink.value = inkColour;
    const autoClear = renderer.autoClear;
    renderer.autoClear = false;
    renderer.render(inkScene, inkCamera);
    renderer.autoClear = autoClear;

    geometry.dispose();
    lines.geometry.dispose();
    ctx.drawImage(renderer.domElement, 0, 0, ctx.canvas.width, ctx.canvas.height);
};
