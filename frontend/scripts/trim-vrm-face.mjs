#!/usr/bin/env node
// Cuts a VRM 1.0 avatar down to the one thing the expression reference uses:
// the face. See .design/expression-reference.md for why.
//
//   node scripts/trim-vrm-face.mjs <in.vrm> <out.vrm>
//
// Kept: the mesh the expression presets drive (the face), its materials and
// textures, the skeleton (the face is skinned to it, and the camera aims at
// the head bone), the humanoid/lookAt/expression definitions and the meta
// with its licence. Dropped: every other mesh (body, clothes, hair), morph
// targets no expression uses, the thumbnail, spring bones and node
// constraints (physics for hair and clothes that are no longer there).
//
// Dropping the hair is deliberate, not just a size win: hair, clothes and a
// body are what make an image "this character", and an expression reference
// that carries them invites the model to copy a character it was never
// asked for.
import { readFileSync, writeFileSync } from 'node:fs';

const [input, output] = process.argv.slice(2);
if (!input || !output) {
    console.error('usage: trim-vrm-face.mjs <in.vrm> <out.vrm>');
    process.exit(1);
}

// --- GLB in ------------------------------------------------------------------
const glb = readFileSync(input);
if (glb.readUInt32LE(0) !== 0x46546c67) throw new Error('not a GLB file');
const jsonLength = glb.readUInt32LE(12);
const gltf = JSON.parse(glb.subarray(20, 20 + jsonLength).toString('utf8'));
const binStart = 20 + jsonLength + 8;
const bin = glb.subarray(binStart, binStart + glb.readUInt32LE(20 + jsonLength));

const vrm = gltf.extensions?.VRMC_vrm;
if (!vrm) throw new Error('not a VRM 1.0 file (no VRMC_vrm extension)');

// --- which mesh is the face ----------------------------------------------------
// Whatever node the expression presets bind morph targets on. Named "Face" by
// VRoid, but the bindings are the definition, not the name.
const expressions = [
    ...Object.values(vrm.expressions?.preset ?? {}),
    ...Object.values(vrm.expressions?.custom ?? {}),
];
const faceNodes = new Set(expressions.flatMap((e) => (e.morphTargetBinds ?? []).map((b) => b.node)));
if (faceNodes.size !== 1) throw new Error(`expected expressions on one node, found ${faceNodes.size}`);
const [faceNode] = faceNodes;
const faceMesh = gltf.nodes[faceNode].mesh;

for (const [i, node] of gltf.nodes.entries()) {
    if (i !== faceNode && node.mesh !== undefined) {
        delete node.mesh;
        delete node.skin;
        delete node.weights;
    }
    if (node.extensions) {
        delete node.extensions.VRMC_node_constraint;
        if (Object.keys(node.extensions).length === 0) delete node.extensions;
    }
}

// --- only the morph targets an expression uses ---------------------------------
const usedTargets = [...new Set(expressions.flatMap((e) => (e.morphTargetBinds ?? []).map((b) => b.index)))].sort((a, b) => a - b);
const targetIndex = new Map(usedTargets.map((old, i) => [old, i]));
const mesh = gltf.meshes[faceMesh];
for (const primitive of mesh.primitives) {
    if (primitive.targets) primitive.targets = usedTargets.map((t) => primitive.targets[t]);
    if (primitive.extras?.targetNames) primitive.extras.targetNames = usedTargets.map((t) => primitive.extras.targetNames[t]);
}
if (mesh.weights) mesh.weights = usedTargets.map((t) => mesh.weights[t]);
if (mesh.extras?.targetNames) mesh.extras.targetNames = usedTargets.map((t) => mesh.extras.targetNames[t]);
for (const e of expressions) for (const b of e.morphTargetBinds ?? []) b.index = targetIndex.get(b.index);

// --- VRM-level references --------------------------------------------------------
delete vrm.meta.thumbnailImage;
if (vrm.firstPerson?.meshAnnotations) {
    vrm.firstPerson.meshAnnotations = vrm.firstPerson.meshAnnotations.filter((a) => a.node === faceNode);
}
delete gltf.extensions.VRMC_springBone;
gltf.extensionsUsed = (gltf.extensionsUsed ?? []).filter((e) => e !== 'VRMC_springBone' && e !== 'VRMC_node_constraint');

// --- garbage-collect everything nothing points at any more ---------------------
// Each pass marks what is still referenced, then renumbers it; later passes
// read the renumbered indices.
const compact = (list, used, rewrite) => {
    const keep = [...used].sort((a, b) => a - b);
    const map = new Map(keep.map((old, i) => [old, i]));
    rewrite((old) => map.get(old));
    return keep.map((i) => list[i]);
};

// meshes and skins, referenced from nodes
gltf.meshes = compact(gltf.meshes, new Set(gltf.nodes.flatMap((n) => (n.mesh === undefined ? [] : [n.mesh]))), (m) => {
    for (const n of gltf.nodes) if (n.mesh !== undefined) n.mesh = m(n.mesh);
});
gltf.skins = compact(gltf.skins ?? [], new Set(gltf.nodes.flatMap((n) => (n.skin === undefined ? [] : [n.skin]))), (m) => {
    for (const n of gltf.nodes) if (n.skin !== undefined) n.skin = m(n.skin);
});

// materials, referenced from primitives and expression material binds
const materialRefs = () => [
    ...gltf.meshes.flatMap((me) => me.primitives.filter((p) => p.material !== undefined).map((p) => ({ o: p, k: 'material' }))),
    ...expressions.flatMap((e) => [...(e.materialColorBinds ?? []), ...(e.textureTransformBinds ?? [])].map((b) => ({ o: b, k: 'material' }))),
];
gltf.materials = compact(gltf.materials ?? [], new Set(materialRefs().map(({ o, k }) => o[k])), (m) => {
    for (const { o, k } of materialRefs()) o[k] = m(o[k]);
});

// textures, referenced from anything in a material shaped `somethingTexture: { index }`
const textureRefs = () => {
    const found = [];
    const walk = (value, key) => {
        if (!value || typeof value !== 'object') return;
        if (/texture$/i.test(key ?? '') && typeof value.index === 'number') found.push(value);
        for (const [k, v] of Object.entries(value)) walk(v, k);
    };
    for (const material of gltf.materials) walk(material);
    return found;
};
gltf.textures = compact(gltf.textures ?? [], new Set(textureRefs().map((t) => t.index)), (m) => {
    for (const t of textureRefs()) t.index = m(t.index);
});

// images and samplers, referenced from textures
const textureSource = (t) => t.extensions?.EXT_texture_webp?.source ?? t.source;
gltf.images = compact(gltf.images ?? [], new Set(gltf.textures.map(textureSource)), (m) => {
    for (const t of gltf.textures) {
        if (t.source !== undefined) t.source = m(t.source);
        if (t.extensions?.EXT_texture_webp) t.extensions.EXT_texture_webp.source = m(t.extensions.EXT_texture_webp.source);
    }
});
gltf.samplers = compact(gltf.samplers ?? [], new Set(gltf.textures.filter((t) => t.sampler !== undefined).map((t) => t.sampler)), (m) => {
    for (const t of gltf.textures) if (t.sampler !== undefined) t.sampler = m(t.sampler);
});

// accessors, referenced from primitives and skins
const accessorRefs = () => {
    const refs = [];
    for (const me of gltf.meshes) {
        for (const p of me.primitives) {
            for (const k of Object.keys(p.attributes)) refs.push({ o: p.attributes, k });
            if (p.indices !== undefined) refs.push({ o: p, k: 'indices' });
            for (const target of p.targets ?? []) for (const k of Object.keys(target)) refs.push({ o: target, k });
        }
    }
    for (const s of gltf.skins) if (s.inverseBindMatrices !== undefined) refs.push({ o: s, k: 'inverseBindMatrices' });
    return refs;
};
gltf.accessors = compact(gltf.accessors ?? [], new Set(accessorRefs().map(({ o, k }) => o[k])), (m) => {
    for (const { o, k } of accessorRefs()) o[k] = m(o[k]);
});

// buffer views, referenced from accessors (incl. sparse) and images — then the
// binary chunk is rebuilt from only those, each aligned to 4 bytes.
const viewRefs = () => {
    const refs = [];
    for (const a of gltf.accessors) {
        if (a.bufferView !== undefined) refs.push({ o: a, k: 'bufferView' });
        if (a.sparse) refs.push({ o: a.sparse.indices, k: 'bufferView' }, { o: a.sparse.values, k: 'bufferView' });
    }
    for (const im of gltf.images) if (im.bufferView !== undefined) refs.push({ o: im, k: 'bufferView' });
    return refs;
};
gltf.bufferViews = compact(gltf.bufferViews, new Set(viewRefs().map(({ o, k }) => o[k])), (m) => {
    for (const { o, k } of viewRefs()) o[k] = m(o[k]);
});
const chunks = [];
let offset = 0;
for (const view of gltf.bufferViews) {
    const data = bin.subarray(view.byteOffset ?? 0, (view.byteOffset ?? 0) + view.byteLength);
    const pad = (4 - (offset % 4)) % 4;
    if (pad) chunks.push(Buffer.alloc(pad));
    offset += pad;
    view.byteOffset = offset;
    view.buffer = 0;
    chunks.push(data);
    offset += data.length;
}
const newBin = Buffer.concat(chunks);
gltf.buffers = [{ byteLength: newBin.length }];

// --- GLB out -------------------------------------------------------------------
const pad4 = (buf, fill) => Buffer.concat([buf, Buffer.alloc((4 - (buf.length % 4)) % 4, fill)]);
const jsonChunk = pad4(Buffer.from(JSON.stringify(gltf), 'utf8'), 0x20);
const binChunk = pad4(newBin, 0);
const header = Buffer.alloc(12);
header.writeUInt32LE(0x46546c67, 0);
header.writeUInt32LE(2, 4);
header.writeUInt32LE(12 + 8 + jsonChunk.length + 8 + binChunk.length, 8);
const chunkHeader = (length, type) => {
    const h = Buffer.alloc(8);
    h.writeUInt32LE(length, 0);
    h.writeUInt32LE(type, 4);
    return h;
};
writeFileSync(output, Buffer.concat([
    header,
    chunkHeader(jsonChunk.length, 0x4e4f534a), jsonChunk,
    chunkHeader(binChunk.length, 0x004e4942), binChunk,
]));
console.log(`${input}: ${(glb.length / 1e6).toFixed(2)} MB → ${output}: ${((12 + 16 + jsonChunk.length + binChunk.length) / 1e6).toFixed(2)} MB`);
console.log(`kept mesh "${mesh.name}" with ${usedTargets.length} morph targets, ${gltf.materials.length} materials, ${gltf.images.length} images`);
