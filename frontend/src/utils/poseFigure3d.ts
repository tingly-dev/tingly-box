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
// dozen, and the library alone has thirty-six tiles.
import {
    AmbientLight,
    BufferGeometry,
    CylinderGeometry,
    DirectionalLight,
    Group,
    LatheGeometry,
    Mesh,
    MeshStandardMaterial,
    PerspectiveCamera,
    Quaternion,
    Scene,
    SphereGeometry,
    Vector2,
    Vector3,
    WebGLRenderer,
} from 'three';
import {
    figureSolids,
    figureUnit,
    projectionOf,
    projectPoint,
    toneFor,
    type PoseFigure,
    type Solid,
} from './poseFigure';

// The projection is in canvas pixels with y down; three's is y up. Nothing
// else differs, so the mapping is one sign.
const toScene = (p: { x: number; y: number; z?: number }): Vector3 => new Vector3(p.x, -p.y, p.z ?? 0);
const UP = new Vector3(0, 1, 0);

// One fixed studio light: key up-left-front, a fill from the right, and enough
// ambient that the far side of a limb is still a limb.
const buildLights = (scene: Scene): void => {
    const key = new DirectionalLight(0xffffff, 2.2);
    key.position.set(-1, 1.4, 1.6);
    const fill = new DirectionalLight(0xffffff, 0.7);
    fill.position.set(1.2, 0.3, 0.8);
    scene.add(key, fill, new AmbientLight(0xffffff, 0.55));
};

// Shapes that never change are built once at unit size and scaled per mesh.
// Cylinders can't be — a taper is not a scale of another taper — so those are
// built per draw and disposed with it.
const unitSphere = new SphereGeometry(1, 32, 24);
const lathes = new Map<string, LatheGeometry>();
const latheFor = (profile: readonly (readonly [number, number])[]): LatheGeometry => {
    const key = profile.map(([h, w]) => `${h}:${w}`).join(',');
    let geometry = lathes.get(key);
    if (!geometry) {
        geometry = new LatheGeometry(profile.map(([h, w]) => new Vector2(Math.max(w, 0.002), h)), 48);
        lathes.set(key, geometry);
    }
    return geometry;
};
const materials = new Map<string, MeshStandardMaterial>();
const materialFor = (hex: string): MeshStandardMaterial => {
    let material = materials.get(hex);
    if (!material) {
        material = new MeshStandardMaterial({ color: hex, roughness: 0.62, metalness: 0 });
        materials.set(hex, material);
    }
    return material;
};

const meshesFor = (figure: PoseFigure, hex: string): { group: Group; disposable: BufferGeometry[] } => {
    const group = new Group();
    const material = materialFor(hex);
    const disposable: BufferGeometry[] = [];
    const place = (geometry: BufferGeometry, position: Vector3, axis?: Vector3, scale?: Vector3): void => {
        const mesh = new Mesh(geometry, material);
        mesh.position.copy(position);
        if (axis) mesh.quaternion.copy(new Quaternion().setFromUnitVectors(UP, axis));
        if (scale) mesh.scale.copy(scale);
        group.add(mesh);
    };
    for (const solid of figureSolids(figure)) {
        if (solid.kind === 'sphere') {
            const scale = solid.scale
                ? new Vector3(solid.scale.x, solid.scale.y, solid.scale.z ?? solid.scale.x)
                : new Vector3(solid.radius, solid.radius, solid.radius);
            place(unitSphere, toScene(solid.center), solid.axis ? toScene(solid.axis).normalize() : undefined, scale);
        } else if (solid.kind === 'capsule') {
            const from = toScene(solid.from);
            const to = toScene(solid.to);
            const bone = to.clone().sub(from);
            const length = bone.length();
            if (length < 1e-6) continue;
            const geometry = new CylinderGeometry(solid.toRadius, solid.fromRadius, length, 28, 1, false);
            disposable.push(geometry);
            place(geometry, from.clone().addScaledVector(bone, 0.5), bone.normalize());
        } else {
            place(
                latheFor(solid.profile),
                toScene(solid.base),
                toScene(solid.axis).normalize(),
                new Vector3(solid.unit, solid.unit, solid.unit * solid.depth),
            );
        }
    }
    return { group, disposable };
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
    ctx.save();
    ctx.fillStyle = hex;
    ctx.strokeStyle = '#5b6066';
    ctx.lineWidth = Math.max(figureUnit(figure) * 0.006, 1);
    ctx.lineJoin = 'round';
    for (const solid of figureSolids(figure)) {
        ctx.beginPath();
        if (solid.kind === 'sphere') {
            const c = projectPoint(solid.center, projection);
            const r = (solid.scale ? Math.max(solid.scale.x, solid.scale.y) : solid.radius) * c.scale;
            ctx.arc(c.x, c.y, r, 0, Math.PI * 2);
        } else {
            const [a, b, ra, rb] = solid.kind === 'capsule'
                ? [projectPoint(solid.from, projection), projectPoint(solid.to, projection), solid.fromRadius, solid.toRadius]
                : (() => {
                    const top = solid.profile[solid.profile.length - 1][0] * solid.unit;
                    const widest = Math.max(...solid.profile.map(([, w]) => w)) * solid.unit;
                    const end = { x: solid.base.x + solid.axis.x * top, y: solid.base.y + solid.axis.y * top, z: (solid.base.z ?? 0) + (solid.axis.z ?? 0) * top };
                    return [projectPoint(solid.base, projection), projectPoint(end, projection), widest, widest] as const;
                })();
            const angle = Math.atan2(b.y - a.y, b.x - a.x);
            ctx.arc(a.x, a.y, ra * a.scale, angle + Math.PI / 2, angle - Math.PI / 2);
            ctx.arc(b.x, b.y, rb * b.scale, angle - Math.PI / 2, angle + Math.PI / 2);
            ctx.closePath();
        }
        ctx.fill();
        ctx.stroke();
    }
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
    const { group, disposable } = meshesFor(figure, hex);
    scene.add(group);
    renderer.render(scene, cameraFor(figure, width, height));
    for (const geometry of disposable) geometry.dispose();
    ctx.drawImage(renderer.domElement, 0, 0);
};
