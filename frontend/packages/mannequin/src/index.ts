// The mannequin, as a library: a rigged three-dimensional artist's doll with a
// pose library, a per-figure camera, and a WebGL renderer. Nothing in here
// knows about the sketch canvas or the app — it takes points and sizes and
// hands back figures, pixels and hit results.
export * from './types';
export * from './vec3';
export * from './skeleton';
export * from './camera';
export * from './rig';
export * from './transform';
export * from './view';
export * from './figure';
export * from './poses/spec';
export * from './poses/library';
export * from './body';
export * from './interact';
export * from './render3d';
