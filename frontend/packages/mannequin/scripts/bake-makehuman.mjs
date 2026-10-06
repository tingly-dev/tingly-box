// Bakes the mannequin's white model from MakeHuman's base mesh.
//
//   node scripts/bake-makehuman.mjs <path to an unpacked `makehuman-data` npm package>
//
// Writes `src/model/makehuman.ts`. The input is not vendored (it is 260 MB);
// fetch it with `npm pack makehuman-data@0.0.2` and unpack it anywhere.
//
// What it does, per build:
//   1. Morph the base mesh (hm08) with MakeHuman's own macro targets: the
//      race-averaged young female / male shape, plus "ideal proportions" —
//      MakeHuman's artistic-canon proportions, which is what a drawing
//      mannequin wants rather than an average person.
//   2. Keep only the body surface (no helper geometry, eyes, teeth, joints).
//   3. Read the skeleton off MakeHuman's joint helpers, in our sixteen joints.
//   4. Collapse MakeHuman's 163-bone skin weights onto the twelve segments our
//      rig moves (pelvis, ribcage, neck, head, and two per limb).
//   5. Stylise it into an art mannequin and cut its construction lines.
// Positions are quantised to int16 within the bounding box, so the module is
// a few hundred kilobytes.
//
// Licence: MakeHuman's assets (base mesh, targets, skeleton) were dedicated to
// the public domain (CC0 1.0) by the MakeHuman team from 1.1.0 on; the data
// here is that mesh. MakeHuman's *code* is AGPL and none of it is used.
import fs from 'node:fs';
import path from 'node:path';

const root = process.argv[2];
if (!root) throw new Error('usage: node scripts/bake-makehuman.mjs <makehuman-data package dir>');
const out = path.join(path.dirname(new URL(import.meta.url).pathname), '../src/model/makehuman.ts');

const human = JSON.parse(fs.readFileSync(path.join(root, 'public/data/models/human_full_size.json'), 'utf8'));
const targetKeys = Object.keys(JSON.parse(fs.readFileSync(path.join(root, 'src/json/targets/target-list.json'), 'utf8')).targets).sort();
const N = human.vertices.length;
const bin = fs.openSync(path.join(root, 'public/data/targets/targets.bin'), 'r');
const targetRow = (name) => {
    const index = targetKeys.indexOf(`data/targets/${name}.target`);
    if (index < 0) throw new Error(`no target ${name}`);
    const buffer = Buffer.alloc(N * 2);
    fs.readSync(bin, buffer, 0, N * 2, index * N * 2);
    return new Int16Array(buffer.buffer, buffer.byteOffset, N);
};

// --- faces (three.js JSON 3.1) ---------------------------------------------
const materials = human.materials.map((m) => m.DbgName);
const keepMaterial = new Set(['body']);
const faces = [];
{
    const f = human.faces;
    const uvLayers = human.uvs.length;
    let i = 0;
    while (i < f.length) {
        const type = f[i++];
        const quad = type & 1;
        const nv = quad ? 4 : 3;
        const verts = f.slice(i, i + nv); i += nv;
        let material = 0;
        if (type & 2) material = f[i++];
        if (type & 4) i += uvLayers;
        if (type & 8) i += nv * uvLayers;
        if (type & 16) i += 1;
        if (type & 32) i += nv;
        if (type & 64) i += 1;
        if (type & 128) i += nv;
        if (keepMaterial.has(materials[material])) faces.push(verts);
    }
}

// --- morphing ---------------------------------------------------------------
const morph = (weights) => {
    const v = Float64Array.from(human.vertices);
    for (const [name, w] of weights) {
        const row = targetRow(name);
        for (let k = 0; k < N; k += 1) v[k] += row[k] * 1e-3 * w;
    }
    return v;
};
const race = (gender) => ['african', 'asian', 'caucasian'].map((r) => [`macrodetails/${r}-${gender}-young`, 1 / 3]);
// A mannequin, not a nude: the nipples are taken down to MakeHuman's minimum
// and pressed in, so the chest reads as form. It is also what keeps a sketch
// built on this model from looking like a photograph of an undressed person
// to the image model it is sent to.
const MANNEQUIN = [['breast/nipple-size-min', 1], ['breast/nipple-point-in', 1]];
// Moderate stylisation, the way figure-drawing mannequins push a body: the
// female a clearer hourglass (a little more bust and hip, a narrower waist,
// a flat belly), the male a clearer wedge (broader shoulders, more V, a
// firmer chest). One strength for all of it, so "how stylised" is one number
// to turn — 0 is MakeHuman's ideal proportions as they are. Kept moderate on
// purpose: the sketch goes to an image model, and a strongly sexualised
// silhouette is both what moderation trips on and what would bend every
// pose in a library shared by both builds.
const STYLE = 1.35;
// A drawing mannequin's head is drawn a size up from life — it is where the
// eye goes first, and the Loomis lines need room. The egg grows about its
// chin, so the neck it sits on does not change.
const HEAD = 1.12;
const stylised = (pairs) => pairs.map(([target, w]) => [target, w * STYLE]);
const FEMININE = stylised([
    ['breast/female-young-averagemuscle-averageweight-maxcup-averagefirmness', 0.28],
    ['breast/female-young-averagemuscle-averageweight-averagecup-maxfirmness', 0.3],
    ['measure/measure-waist-decrease', 0.5],
    ['measure/measure-hips-increase', 0.3],
    ['buttocks/buttocks-volume-incr', 0.35],
    ['stomach/stomach-pregnant-decr', 0.5],
]);
const MASCULINE = stylised([
    ['torso/torso-vshape-more', 0.5],
    ['measure/measure-shoulder-increase', 0.3],
    ['measure/measure-waist-decrease', 0.3],
    ['torso/torso-muscle-pectoral-incr', 0.3],
    ['stomach/stomach-pregnant-decr', 0.5],
]);
const BUILDS = {
    female: morph([...race('female'), ['macrodetails/proportions/female-young-averagemuscle-averageweight-idealproportions', 1], ...FEMININE, ...MANNEQUIN]),
    male: morph([...race('male'), ['macrodetails/proportions/male-young-averagemuscle-averageweight-idealproportions', 1], ...MASCULINE, ...MANNEQUIN]),
};

// --- skeleton ---------------------------------------------------------------
// Our L is the figure's right (picture-left when it faces you), so it is
// MakeHuman's `.R`. MakeHuman is y-up and faces +z; we are y-down, so y flips.
const jointIdx = human.metadata.joint_pos_idxs;
const at = (v, key) => {
    const idx = jointIdx[key];
    if (!idx) throw new Error(`no joint ${key}`);
    const p = [0, 0, 0];
    for (const i of idx) for (let k = 0; k < 3; k += 1) p[k] += v[i * 3 + k] / idx.length;
    return [p[0], -p[1], p[2]];
};
const mid = (a, b) => a.map((x, k) => (x + b[k]) / 2);
const sub = (a, b) => a.map((x, k) => x - b[k]);
const len = (a) => Math.hypot(...a);
const skeletonOf = (v) => {
    const J = {
        hipL: at(v, 'upperleg01.R____head'), hipR: at(v, 'upperleg01.L____head'),
        kneeL: at(v, 'lowerleg01.R____head'), kneeR: at(v, 'lowerleg01.L____head'),
        ankleL: at(v, 'foot.R____head'), ankleR: at(v, 'foot.L____head'),
        shoulderL: at(v, 'upperarm01.R____head'), shoulderR: at(v, 'upperarm01.L____head'),
        elbowL: at(v, 'lowerarm01.R____head'), elbowR: at(v, 'lowerarm01.L____head'),
        wristL: at(v, 'wrist.R____head'), wristR: at(v, 'wrist.L____head'),
        neck: at(v, 'neck01____head'),
        // Where the spine bends: the top of the lumbar curve, between the
        // pelvis and the ribcage. One joint here is what lets a pose have a
        // gesture — an arch, a slump, a side-bend — instead of a plank.
        chest: at(v, 'spine02____head'),
        head: mid(at(v, 'head____head'), at(v, 'head____tail')),
    };
    // The root sits a little above the femoral heads, on the spine, which is
    // how our skeleton hangs the hip line off it.
    const hips = mid(J.hipL, J.hipR);
    J.hip = [hips[0], hips[1] - 0.022 * (len(sub(J.neck, hips)) / 0.36), hips[2]];
    return J;
};

// --- skin weights onto our twelve segments ----------------------------------
const SEGMENTS = ['pelvis', 'chest', 'neck', 'head', 'upperArmL', 'foreArmL', 'upperArmR', 'foreArmR', 'thighL', 'shinL', 'thighR', 'shinR'];
const seg = (name) => SEGMENTS.indexOf(name);
const ours = (side) => (side === 'R' ? 'L' : 'R');
const boneShare = (name) => {
    const base = name.replace(/____head$|____tail$/, '');
    const side = /\.([LR])$/.exec(base)?.[1];
    const s = side ? ours(side) : null;
    const b = base.replace(/\.[LR]$/, '');
    if (b === 'root' || b === 'spine05' || b === 'pelvis') return [[seg('pelvis'), 1]];
    // The ribcage turns about the waist joint (`spine02`), so the lumbar bones
    // below it go with the pelvis and the blend is centred on it.
    if (b === 'spine04') return [[seg('pelvis'), 1]];
    if (b === 'spine03') return [[seg('pelvis'), 0.7], [seg('chest'), 0.3]];
    if (b === 'spine02') return [[seg('pelvis'), 0.2], [seg('chest'), 0.8]];
    if (b === 'spine01' || b === 'breast' || b === 'clavicle') return [[seg('chest'), 1]];
    if (b === 'shoulder01') return [[seg('chest'), 0.5], [seg(`upperArm${s}`), 0.5]];
    if (/^neck0[12]$/.test(b)) return [[seg('neck'), 1]];
    if (b === 'neck03') return [[seg('neck'), 0.5], [seg('head'), 0.5]];
    if (/^upperarm0[12]$/.test(b)) return [[seg(`upperArm${s}`), 1]];
    if (/^lowerarm0[12]$/.test(b) || /^(wrist|metacarpal|finger)/.test(b)) return [[seg(`foreArm${s}`), 1]];
    if (/^upperleg0[12]$/.test(b)) return [[seg(`thigh${s}`), 1]];
    if (/^lowerleg0[12]$/.test(b) || /^(foot|toe)/.test(b)) return [[seg(`shin${s}`), 1]];
    // Everything else is the face and skull: eyes, jaw, tongue, the lot.
    return [[seg('head'), 1]];
};
const boneNames = human.bones.map((b) => b.name);
const shares = boneNames.map(boneShare);

// --- keep only the body's vertices ------------------------------------------
const used = new Map();
const order = [];
for (const f of faces) for (const v of f) if (!used.has(v)) { used.set(v, order.length); order.push(v); }
const triangles = [];
for (const f of faces) {
    const [a, b, c, d] = f.map((v) => used.get(v));
    triangles.push(a, b, c);
    if (d !== undefined) triangles.push(a, c, d);
}
const M = order.length;

const influences = human.influencesPerVertex;
const segIdx = new Uint8Array(M * 4);
const segW = new Uint8Array(M * 4);
for (let i = 0; i < M; i += 1) {
    const v = order[i];
    const acc = new Float64Array(SEGMENTS.length);
    for (let k = 0; k < influences; k += 1) {
        const bone = human.skinIndices[v * influences + k];
        const w = human.skinWeights[v * influences + k];
        for (const [s, share] of shares[bone]) acc[s] += w * share;
    }
    const top = [...acc.keys()].sort((x, y) => acc[y] - acc[x]).slice(0, 4);
    const total = top.reduce((sum, s) => sum + acc[s], 0) || 1;
    let left = 255;
    top.forEach((s, k) => {
        const q = k === 3 ? left : Math.round((acc[s] / total) * 255);
        segIdx[i * 4 + k] = s;
        segW[i * 4 + k] = Math.max(0, Math.min(left, q));
        left -= segW[i * 4 + k];
    });
}

// --- stylising: from a person to an art mannequin -----------------------------
//
// The reference is a drawing mannequin, not a person: the head an egg with no
// features, the skin without its anatomy-book detail, hands and feet simple.
// All of it is done to the mesh here, once, so the runtime stays a plain skin.
const neighbours = Array.from({ length: M }, () => new Set());
for (let t = 0; t < triangles.length; t += 3) {
    const [a, b, c] = [triangles[t], triangles[t + 1], triangles[t + 2]];
    neighbours[a].add(b).add(c); neighbours[b].add(a).add(c); neighbours[c].add(a).add(b);
}
const adjacency = neighbours.map((n) => [...n]);
const weightOf = (i, segment) => {
    let w = 0;
    for (let k = 0; k < 4; k += 1) if (segIdx[i * 4 + k] === segment) w += segW[i * 4 + k] / 255;
    return w;
};
const dominant = (i) => segIdx[i * 4];

// Taubin smoothing (λ|μ): takes detail off without the shrinkage plain
// Laplacian smoothing has. `mask` weighs how much each vertex may move.
const taubin = (pos, iterations, mask) => {
    const step = (factor) => {
        const next = Float64Array.from(pos);
        for (let i = 0; i < M; i += 1) {
            const m = mask ? mask[i] : 1;
            if (m <= 0) continue;
            const n = adjacency[i];
            for (let k = 0; k < 3; k += 1) {
                let mean = 0;
                for (const j of n) mean += pos[j * 3 + k];
                mean /= n.length;
                next[i * 3 + k] = pos[i * 3 + k] + factor * m * (mean - pos[i * 3 + k]);
            }
        }
        pos.set(next);
    };
    for (let it = 0; it < iterations; it += 1) { step(0.5); step(-0.53); }
};

const eggs = new Map();
const stylise = (pos, joints) => {
    const at = (i) => [pos[i * 3], pos[i * 3 + 1], pos[i * 3 + 2]];
    const head = seg('head');
    const center = joints.head;
    // The head becomes a Loomis egg: a cranium ellipsoid over a tapering jaw,
    // sized off the model's own crown and chin, and every head vertex is
    // carried out (or in) along its ray from the egg's centre onto that
    // surface. Eyes, nose, mouth and ears are gone; the skull's mass and the
    // jaw's wedge stay. Blended by skin weight, so the neck is untouched.
    const onHead = new Float64Array(M);
    for (let i = 0; i < M; i += 1) onHead[i] = Math.max(0, Math.min(1, (weightOf(i, head) - 0.3) / 0.55));
    let crownY = Infinity, chinY = -Infinity;
    for (let i = 0; i < M; i += 1) {
        const p = at(i);
        crownY = Math.min(crownY, p[1]);
        if (weightOf(i, head) > 0.94 && p[2] > center[2]) chinY = Math.max(chinY, p[1]);
    }
    const L = chinY - crownY;
    // The egg's own axes: up along the head bone (the model's neck leans
    // forward, and an egg stood bolt upright on it reads as looking up),
    // forward square to it, across completing the frame.
    const upAxis = (() => { const d = sub(center, joints.neck); const l = len(d); return [d[0] / l, d[1] / l, d[2] / l]; })();
    const fwd = (() => { const f = [0, 0, 1]; const k = f[0] * upAxis[0] + f[1] * upAxis[1] + f[2] * upAxis[2]; const g = f.map((x, i) => x - upAxis[i] * k); const l = len(g); return g.map((x) => x / l); })();
    const side = [upAxis[1] * fwd[2] - upAxis[2] * fwd[1], upAxis[2] * fwd[0] - upAxis[0] * fwd[2], upAxis[0] * fwd[1] - upAxis[1] * fwd[0]];
    // Centred halfway between crown and chin, which is also where Loomis
    // puts the eye line.
    const mid = (crownY + chinY) / 2;
    const t = (mid - center[1]) / upAxis[1];
    const lift = (HEAD - 1) * (L / 2);
    const origin = [center[0] + upAxis[0] * (t + lift), mid + upAxis[1] * lift, center[2] + upAxis[2] * (t + lift) + 0.01 * L];
    const half = (L / 2) * HEAD;
    const A = 0.40 * L * HEAD, FRONT = 0.46 * L * HEAD, BACK = 0.52 * L * HEAD;
    const inside = (u, v, w) => {
        // v is up the head, w forward, u across.
        const s = v < 0 ? Math.min(1, -v / half) : 0;
        // Loomis: the jaw is about seven-tenths the skull's width and the chin
        // narrower still; the face plane drops nearly straight; the back of
        // the skull rounds under onto the neck.
        const a = A * (1 - 0.5 * s ** 1.9);
        const front = FRONT * (1 - 0.22 * s ** 1.6);
        const back = BACK * (1 - 0.72 * s ** 1.3);
        const depth = w >= 0 ? front : back;
        return (u / a) ** 2 + (v / half) ** 2 + (w / depth) ** 2;
    };
    const dotv = (x, y) => x[0] * y[0] + x[1] * y[1] + x[2] * y[2];
    // `upAxis` points from neck to head in our y-down space, i.e. "up" the
    // body is its direction; v is measured along it.
    const onEgg = (p) => {
        const d = sub(p, origin);
        const l = len(d) || 1;
        const dir = d.map((x) => x / l);
        const du = dotv(dir, side), dv = dotv(dir, upAxis), dw = dotv(dir, fwd);
        let lo = 0, hi = L;
        for (let it = 0; it < 32; it += 1) {
            const m = (lo + hi) / 2;
            if (inside(du * m, dv * m, dw * m) < 1) lo = m; else hi = m;
        }
        return origin.map((x, k) => x + dir[k] * lo);
    };
    const original = Float64Array.from(pos);
    const blend = Float64Array.from(onHead, (h) => h * h * (3 - 2 * h));
    const place = (i, target) => {
        for (let k = 0; k < 3; k += 1) pos[i * 3 + k] = original[i * 3 + k] * (1 - blend[i]) + target[k] * blend[i];
    };
    for (let i = 0; i < M; i += 1) if (blend[i] > 0) place(i, onEgg(at(i)));
    // Projection keeps the old topology, so the mouth, the eye sockets and
    // the ears collapse onto the egg as overlapping folds. Relaxing the
    // vertices across the egg — average with the neighbours, back onto the
    // surface, repeat — spreads those folds out into an even skin.
    for (let it = 0; it < 160; it += 1) {
        const next = Float64Array.from(pos);
        for (let i = 0; i < M; i += 1) {
            if (blend[i] < 0.999) continue;
            const mean = [0, 0, 0];
            for (const j of adjacency[i]) for (let k = 0; k < 3; k += 1) mean[k] += pos[j * 3 + k] / adjacency[i].length;
            const target = onEgg(mean);
            for (let k = 0; k < 3; k += 1) next[i * 3 + k] = target[k];
        }
        pos.set(next);
    }
    const egg = { origin, upAxis, side, fwd, L: L * HEAD };
    eggs.set(pos, egg);
    taubin(pos, 25, onHead);
    // Hands and feet: simplified to mittens and socks — the fingers and toes
    // stay readable as a hand and a foot, without their nails and knuckles.
    const extremity = new Float64Array(M);
    for (let i = 0; i < M; i += 1) {
        const p = at(i);
        let e = 0;
        for (const side of ['L', 'R']) {
            const wrist = joints[`wrist${side}`], elbow = joints[`elbow${side}`];
            const ankle = joints[`ankle${side}`], knee = joints[`knee${side}`];
            const along = (from, to) => {
                const axis = sub(to, from);
                const l = len(axis);
                return (sub(p, to).reduce((sum, x, k) => sum + x * axis[k], 0) / l) / 0.03;
            };
            if (dominant(i) === seg(`foreArm${side}`)) e = Math.max(e, Math.min(1, along(elbow, wrist)));
            if (dominant(i) === seg(`shin${side}`)) e = Math.max(e, Math.min(1, along(knee, ankle)));
        }
        extremity[i] = Math.max(0, e);
    }
    taubin(pos, 40, extremity);
    // The root of the neck — the trapezius and the collarbones — is where
    // the base mesh is most detailed and the ink line most likely to catch.
    const collar = new Float64Array(M);
    for (let i = 0; i < M; i += 1) collar[i] = Math.max(0, 1 - len(sub(at(i), joints.neck)) / 0.11);
    taubin(pos, 20, collar);
    // And everything, lightly: skin detail out, masses in.
    taubin(pos, 10);
};

// --- the mannequin's construction lines --------------------------------------
//
// Rings round the joints, like the seams on a jointed doll, and the head's
// centre and eye lines, as in every Loomis head. Each is the cut of the rest
// mesh by a plane, kept as segments between points on mesh edges (two vertex
// indices and a fraction), so at runtime they ride the skin exactly.
const cutLines = (pos, joints) => {
    const at = (i) => [pos[i * 3], pos[i * 3 + 1], pos[i * 3 + 2]];
    const norm = (a) => { const l = len(a); return a.map((x) => x / l); };
    const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
    const add = (a, b) => a.map((x, k) => x + b[k]);
    const scale = (a, f) => a.map((x) => x * f);
    const rings = [];
    const ring = (point, normal, radius, segments, keep = () => true) => rings.push({ point, normal: norm(normal), radius, segments: new Set(segments.map(seg)), keep });
    const up = norm(sub(joints.neck, joints.hip));
    ring(add(joints.neck, scale(up, -0.012)), sub(joints.head, joints.neck), 0.08, ['chest', 'neck']);
    ring(add(joints.hip, scale(up, 0.15)), up, 0.3, ['chest', 'pelvis']);
    for (const side of ['L', 'R']) {
        const S = joints[`shoulder${side}`], E = joints[`elbow${side}`], W = joints[`wrist${side}`];
        const H = joints[`hip${side}`], K = joints[`knee${side}`], A = joints[`ankle${side}`];
        const upper = norm(sub(E, S)), fore = norm(sub(W, E)), thigh = norm(sub(K, H)), shin = norm(sub(A, K));
        ring(add(S, scale(upper, 0.012)), upper, 0.085, ['chest', `upperArm${side}`, `foreArm${side}`]);
        ring(E, add(upper, fore), 0.06, [`upperArm${side}`, `foreArm${side}`]);
        ring(W, fore, 0.05, [`foreArm${side}`]);
        ring(add(H, scale(thigh, 0.02)), add(thigh, scale(up, -0.6)), 0.12, ['pelvis', `thigh${side}`]);
        ring(K, add(thigh, shin), 0.08, [`thigh${side}`, `shin${side}`]);
        ring(A, shin, 0.06, [`shin${side}`]);
    }
    // The torso's own construction, as in the reference sheets: a centre line
    // down the front from the pit of the neck to the crotch and one down the
    // back along the spine, and the line under the chest masses.
    const front = norm(sub(joints.shoulderR, joints.shoulderL));
    const midline = joints.chest ?? joints.hip;
    const torsoFront = (p) => p[2] > midline[2] - 0.004;
    ring(midline, front, 0.3, ['chest', 'pelvis'], (p) => torsoFront(p) && p[1] > joints.neck[1] + 0.02);
    ring(midline, front, 0.3, ['chest', 'pelvis'], (p) => !torsoFront(p) && p[1] > joints.neck[1] + 0.04 && p[1] < joints.hip[1] + 0.02);
    const underChest = add(joints.chest, scale(norm(sub(joints.neck, joints.chest)), 0.35 * len(sub(joints.neck, joints.chest))));
    ring(underChest, add(up, [0, 0, 0.35]), 0.2, ['chest'], (p) => p[2] > underChest[2] + 0.01);
    // The ball joints, drawn as the little circles a mannequin sheet puts on
    // them: a cap cut off the front of each knee, the back of each elbow and
    // the outside of each shoulder.
    for (const side of ['L', 'R']) {
        const outward = side === 'L' ? -1 : 1;
        const K = joints[`knee${side}`], E = joints[`elbow${side}`], S = joints[`shoulder${side}`];
        ring(add(K, [0, 0, 0.022]), [0, 0, 1], 0.05, [`thigh${side}`, `shin${side}`]);
        ring(add(E, [0, 0, -0.016]), [0, 0, -1], 0.04, [`upperArm${side}`, `foreArm${side}`]);
        ring(add(S, [outward * 0.022, -0.006, 0]), [outward, -0.25, 0], 0.05, ['chest', `upperArm${side}`]);
    }
    // The head: a centre line front to back over the crown, and the eye line
    // across the front. The front is +z.
    const across = norm(sub(joints.shoulderR, joints.shoulderL));
    const egg = eggs.get(pos);
    const ahead = (p) => dot(sub(p, egg.origin), egg.fwd);
    ring(egg.origin, egg.upAxis, 0.12, ['head'], (p) => ahead(p) > 0);
    ring(egg.origin, egg.side, 0.12, ['head'], (p) => dot(sub(p, egg.origin), egg.upAxis) > 0 || ahead(p) > 0);

    const a = [], b = [], t = [];
    for (const r of rings) {
        const inside = (i) => r.segments.has(dominant(i)) && len(sub(at(i), r.point)) < r.radius && r.keep(at(i));
        const side = (i) => dot(sub(at(i), r.point), r.normal);
        for (let k = 0; k < triangles.length; k += 3) {
            const tri = [triangles[k], triangles[k + 1], triangles[k + 2]];
            if (!tri.every(inside)) continue;
            const crossings = [];
            for (let e = 0; e < 3; e += 1) {
                const i = tri[e], j = tri[(e + 1) % 3];
                const si = side(i), sj = side(j);
                if ((si < 0) !== (sj < 0)) crossings.push([i, j, si / (si - sj)]);
            }
            if (crossings.length !== 2) continue;
            for (const [i, j, f] of crossings) { a.push(i); b.push(j); t.push(Math.round(f * 255)); }
        }
    }
    return { a: encode(new Uint16Array(a)), b: encode(new Uint16Array(b)), t: encode(new Uint8Array(t)) };
};

// --- per build: positions in our units, hip at the origin --------------------
const encode = (array) => Buffer.from(array.buffer, array.byteOffset, array.byteLength).toString('base64');
const builds = {};
const styled = {};
for (const [name, v] of Object.entries(BUILDS)) {
    const J = skeletonOf(v);
    const unit = len(sub(J.neck, J.hip)) / 0.36;
    const to = (p) => sub(p, J.hip).map((x) => x / unit);
    const joints = Object.fromEntries(Object.entries(J).map(([k, p]) => [k, to(p).map((x) => +x.toFixed(5))]));
    const pos = new Float64Array(M * 3);
    for (let i = 0; i < M; i += 1) {
        const p = to([v[order[i] * 3], -v[order[i] * 3 + 1], v[order[i] * 3 + 2]]);
        pos.set(p, i * 3);
    }
    stylise(pos, joints);
    styled[name] = { pos, joints };
}

// Projecting a head onto an egg turns its insides out: the mouth's lining,
// the eye sockets, the backs of the ears all land on the egg facing inward.
// Those triangles are dropped (in either build — the index is shared), and
// what is left is the egg.
{
    const keep = [];
    for (let t = 0; t < triangles.length; t += 3) {
        const tri = [triangles[t], triangles[t + 1], triangles[t + 2]];
        let flipped = false;
        for (const { pos } of Object.values(styled)) {
            const egg = eggs.get(pos);
            const P = tri.map((i) => [pos[i * 3], pos[i * 3 + 1], pos[i * 3 + 2]]);
            const centroid = [0, 1, 2].map((k) => (P[0][k] + P[1][k] + P[2][k]) / 3);
            if (len(sub(centroid, egg.origin)) > egg.L * 0.62) continue;
            if (!tri.every((i) => weightOf(i, seg('head')) > 0.6)) continue;
            const e1 = sub(P[1], P[0]), e2 = sub(P[2], P[0]);
            // Our space is y-down, so the outward normal is e2 × e1.
            const n = [e2[1] * e1[2] - e2[2] * e1[1], e2[2] * e1[0] - e2[0] * e1[2], e2[0] * e1[1] - e2[1] * e1[0]];
            const radial = sub(centroid, egg.origin);
            if (n[0] * radial[0] + n[1] * radial[1] + n[2] * radial[2] < 0) flipped = true;
        }
        if (!flipped) keep.push(...tri);
    }
    console.log(`dropped ${(triangles.length - keep.length) / 3} inside-out head triangles`);
    triangles.length = 0;
    triangles.push(...keep);
}

for (const [name, { pos, joints }] of Object.entries(styled)) {
    const lines = cutLines(pos, joints);
    const lo = [0, 1, 2].map((k) => Math.min(...pos.filter((_, i) => i % 3 === k)));
    const hi = [0, 1, 2].map((k) => Math.max(...pos.filter((_, i) => i % 3 === k)));
    const q = new Int16Array(M * 3);
    for (let i = 0; i < M * 3; i += 1) {
        const k = i % 3;
        q[i] = Math.round(((pos[i] - lo[k]) / (hi[k] - lo[k])) * 65535 - 32768);
    }
    builds[name] = { joints, headLength: +eggs.get(pos).L.toFixed(5), lo: lo.map((x) => +x.toFixed(6)), hi: hi.map((x) => +x.toFixed(6)), positions: encode(q), lines };
}

const index = M > 65535 ? new Uint32Array(triangles) : new Uint16Array(triangles);
const ts = `// GENERATED by scripts/bake-makehuman.mjs — do not edit by hand.
//
// The mannequin's white model: MakeHuman's base mesh (hm08), morphed to a
// young female and a young male with MakeHuman's "ideal proportions", body
// surface only, stylised into an art mannequin (featureless head, simplified
// hands and feet, smoothed skin) with its construction lines, skinned onto
// our twelve rig segments. MakeHuman assets are
// CC0 1.0 (public domain) from MakeHuman 1.1.0 on — https://www.makehuman.org.
/* eslint-disable */
export const MODEL = ${JSON.stringify({
        vertexCount: M,
        segments: SEGMENTS,
        index: { bits: M > 65535 ? 32 : 16, data: encode(index) },
        skin: { segments: encode(segIdx), weights: encode(segW) },
        builds,
    })} as const;
`;
fs.writeFileSync(out, ts);
console.log(`wrote ${out}: ${M} vertices, ${triangles.length / 3} triangles, ${(ts.length / 1024).toFixed(0)} KB`);
for (const [name, b] of Object.entries(builds)) console.log(name, JSON.stringify(b.joints));
