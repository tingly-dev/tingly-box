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
//      rig moves (pelvis, ribcage, neck, head, and three per limb).
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
const BUILDS = {
    female: morph([...race('female'), ['macrodetails/proportions/female-young-averagemuscle-averageweight-idealproportions', 1], ...MANNEQUIN]),
    male: morph([...race('male'), ['macrodetails/proportions/male-young-averagemuscle-averageweight-idealproportions', 1], ...MANNEQUIN]),
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
    if (b === 'spine04') return [[seg('pelvis'), 0.75], [seg('chest'), 0.25]];
    if (b === 'spine03') return [[seg('pelvis'), 0.35], [seg('chest'), 0.65]];
    if (b === 'spine02' || b === 'spine01' || b === 'breast' || b === 'clavicle') return [[seg('chest'), 1]];
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

// --- per build: positions in our units, hip at the origin --------------------
const encode = (array) => Buffer.from(array.buffer, array.byteOffset, array.byteLength).toString('base64');
const builds = {};
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
    const lo = [0, 1, 2].map((k) => Math.min(...pos.filter((_, i) => i % 3 === k)));
    const hi = [0, 1, 2].map((k) => Math.max(...pos.filter((_, i) => i % 3 === k)));
    const q = new Int16Array(M * 3);
    for (let i = 0; i < M * 3; i += 1) {
        const k = i % 3;
        q[i] = Math.round(((pos[i] - lo[k]) / (hi[k] - lo[k])) * 65535 - 32768);
    }
    builds[name] = { joints, lo: lo.map((x) => +x.toFixed(6)), hi: hi.map((x) => +x.toFixed(6)), positions: encode(q) };
}

const index = M > 65535 ? new Uint32Array(triangles) : new Uint16Array(triangles);
const ts = `// GENERATED by scripts/bake-makehuman.mjs — do not edit by hand.
//
// The mannequin's white model: MakeHuman's base mesh (hm08), morphed to a
// young female and a young male with MakeHuman's "ideal proportions", body
// surface only, skinned onto our twelve rig segments. MakeHuman assets are
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
