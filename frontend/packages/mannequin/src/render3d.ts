// The manikin, drawn. A real 3D render of the solids `figureSolids` lists —
// occlusion, foreshortening and light all come out right because they are
// computed, not faked. The camera is the one `projectionOf` describes, to the
// pixel, so the blue handles drawn on top from `projectFigure` sit exactly on
// the joints they move.
//
// Everything is drawn into one shared WebGL canvas and copied onto the caller's
// 2D context: the sketch surface stays a plain canvas (strokes, export and the
// mock backend never learn that WebGL exists), and one context serves every
// thumbnail in the pose library — browsers cap live WebGL contexts at about a
// dozen, and the library alone has forty-four tiles.
import {
    BackSide,
    BufferAttribute,
    BufferGeometry,
    DirectionalLight,
    Group,
    HemisphereLight,
    Mesh,
    MeshBasicMaterial,
    MeshStandardMaterial,
    PerspectiveCamera,
    Scene,
    Vector2,
    Vector3,
    WebGLRenderer,
} from 'three';
import { LineMaterial } from 'three/examples/jsm/lines/LineMaterial.js';
import { LineSegments2 } from 'three/examples/jsm/lines/LineSegments2.js';
import { LineSegmentsGeometry } from 'three/examples/jsm/lines/LineSegmentsGeometry.js';
import { figureLineEdges, figureSurface, toneFor } from './body';
import { projectionOf, projectPoint } from './camera';
import { figureUnit, type PoseFigure } from './skeleton';


// A figure-drawing studio: a strong key high on the left and in front, a cool
// sky / warm floor fill so the shadow side is still modelled, and a rim from
// behind that separates the silhouette from the paper. The terminator — where
// light turns to shadow — is what shows a form's roundness; flat ambient
// light hides it, which is half of why the old manikin read as plastic.
const buildLights = (scene: Scene): void => {
    const key = new DirectionalLight(0xffffff, 2.4);
    key.position.set(-1.1, 1.5, 1.4);
    const rim = new DirectionalLight(0xffffff, 1.1);
    rim.position.set(1.4, 0.6, -1.2);
    scene.add(key, rim, new HemisphereLight(0xf2f5fa, 0x7d746c, 0.9));
};

// The body is drawn see-through, like a figure drawn on tracing paper: thin
// in the middle of each form and solid toward its edges (a Fresnel falloff),
// so it still reads as a lit, round volume, while the limb behind the torso,
// the far leg, the hand behind the back all show through it. Where forms
// overlap the layers add up and darken, which is exactly the "this is in
// front of that" a pose reference needs. Drawn without depth testing so
// every layer shows; the depth pre-pass below keeps the lines honest.
const XRAY = { centre: 0.3, edge: 0.94 };
const materials = new Map<string, MeshStandardMaterial>();
const materialFor = (hex: string): MeshStandardMaterial => {
    let material = materials.get(hex);
    if (!material) {
        material = new MeshStandardMaterial({
            color: hex, roughness: 0.55, metalness: 0,
            transparent: true, depthTest: false, depthWrite: false,
        });
        material.onBeforeCompile = (shader) => {
            shader.fragmentShader = shader.fragmentShader.replace(
                '#include <opaque_fragment>',
                `#include <opaque_fragment>
                float facing = abs(dot(normalize(normal), normalize(vViewPosition)));
                gl_FragColor.a *= mix(${XRAY.edge.toFixed(2)}, ${XRAY.centre.toFixed(2)}, pow(facing, 1.4));`,
            );
        };
        material.customProgramCacheKey = () => 'xray-body';
        materials.set(hex, material);
    }
    return material;
};

// Depth only: the nearest surface, so the ink can tell what is in front.
const depthOnly = new MeshBasicMaterial({ colorWrite: false });

// The outline is an inverted hull: every surface drawn a second time, pushed
// out along its normals, back faces only, in ink. It gives the silhouette and
// every overlap (an arm across the body, a knee in front of a thigh) the line
// a figure drawing would — and lines are what an image model reads first.
const INK = '#3d4249';
const outlines = new Map<number, MeshBasicMaterial>();
const outlineFor = (thickness: number): MeshBasicMaterial => {
    const key = Math.round(thickness * 100) / 100;
    let material = outlines.get(key);
    if (!material) {
        // Pushed back in depth so it shows at the silhouette and where one
        // form passes in front of another, but not through the shallow
        // concavities of the surface itself (collarbones, the small of the
        // back), where an un-offset hull pokes through as stray marks.
        material = new MeshBasicMaterial({ color: INK, side: BackSide, polygonOffset: true, polygonOffsetFactor: 6, polygonOffsetUnits: 16 });
        material.onBeforeCompile = (shader) => {
            shader.vertexShader = shader.vertexShader.replace(
                '#include <begin_vertex>',
                `vec3 transformed = position + normalize(normal) * ${key.toFixed(2)};`,
            );
        };
        material.customProgramCacheKey = () => `outline-${key}`;
        outlines.set(key, material);
    }
    return material;
};

// The construction lines: the seams at the joints of a jointed mannequin and
// the centre and eye lines of a Loomis head — the lines a figure drawing of a
// mannequin has, and the ones the reference sheets this is drawn after show.
// Screen-space thick lines so they hold their weight at any size, nudged off
// the surface along its normal so the skin does not swallow them.
// Each line is drawn twice, as in a construction drawing: solid where it is
// on the near side of the body, and faint where it runs round behind — so a
// ring reads as a ring, all the way round, through the see-through skin.
const lineMaterials = new Map<string, LineMaterial>();
const lineMaterialFor = (width: number, size: Vector2, hidden: boolean): LineMaterial => {
    const key = `${Math.round(width * 10) / 10}:${hidden}`;
    let material = lineMaterials.get(key);
    if (!material) {
        material = new LineMaterial({
            color: INK, linewidth: Math.round(width * 10) / 10, transparent: true,
            opacity: hidden ? 0.22 : 0.8, depthTest: !hidden, depthWrite: false,
        });
        lineMaterials.set(key, material);
    }
    material.resolution.copy(size);
    return material;
};

const linesFor = (figure: PoseFigure, geometry: BufferGeometry, size: Vector2): { visible: LineSegments2; hidden: LineSegments2; geometry: LineSegmentsGeometry } => {
    const { a, b, t } = figureLineEdges(figure);
    const position = geometry.getAttribute('position');
    const normal = geometry.getAttribute('normal');
    const lift = figureUnit(figure) * 0.0035;
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
    const width = Math.max(1, Math.min(2.2, figureUnit(figure) * 0.0028));
    return {
        visible: new LineSegments2(lines, lineMaterialFor(width, size, false)),
        hidden: new LineSegments2(lines, lineMaterialFor(width * 0.8, size, true)),
        geometry: lines,
    };
};

const meshesFor = (figure: PoseFigure, hex: string, size: Vector2): { group: Group; disposable: { dispose(): void }[] } => {
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
    const lines = linesFor(figure, geometry, size);
    // In order: the nearest surface's depth; the see-through body over
    // everything; the hidden halves of the lines; the silhouette ink and
    // the near halves of the lines, both tested against that depth.
    const passes: [Mesh | LineSegments2, number][] = [
        [new Mesh(geometry, depthOnly), 0],
        [new Mesh(geometry, materialFor(hex)), 1],
        [lines.hidden, 2],
        [new Mesh(geometry, outlineFor(Math.max(figureUnit(figure) * 0.0024, 0.6))), 3],
        [lines.visible, 4],
    ];
    const group = new Group();
    for (const [object, order] of passes) {
        object.renderOrder = order;
        group.add(object);
    }
    return { group, disposable: [geometry, lines.geometry] };
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
    const width = ctx.canvas.width;
    const height = ctx.canvas.height;
    const renderer = rendererFor(width, height);
    if (!renderer) {
        drawFlat(ctx, figure, hex);
        return;
    }
    const scene = new Scene();
    buildLights(scene);
    const { group, disposable } = meshesFor(figure, hex, new Vector2(width, height));
    scene.add(group);
    renderer.render(scene, cameraFor(figure, width, height));
    for (const geometry of disposable) geometry.dispose();
    ctx.drawImage(renderer.domElement, 0, 0);
};
