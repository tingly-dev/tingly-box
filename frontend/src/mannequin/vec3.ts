// Three-dimensional vectors, as plain objects. Joints live in world space:
// x/y are canvas pixels, z is depth in the same unit, positive toward the
// viewer. A `Vec3` is structurally a 2D point with a depth, so anything that
// only wants x/y can take one.
export interface Vec3 { x: number; y: number; z: number }

// Depth is read defensively everywhere it is read: a sketch saved before the
// mannequin had a third dimension comes back with flat joints.
export const zOf = (point: Vec3): number => point.z ?? 0;

export const sub3 = (a: Vec3, b: Vec3): Vec3 => ({ x: a.x - b.x, y: a.y - b.y, z: zOf(a) - zOf(b) });
export const add3 = (a: Vec3, b: Vec3): Vec3 => ({ x: a.x + b.x, y: a.y + b.y, z: zOf(a) + zOf(b) });
export const mul3 = (a: Vec3, factor: number): Vec3 => ({ x: a.x * factor, y: a.y * factor, z: zOf(a) * factor });
export const len3 = (a: Vec3): number => Math.hypot(a.x, a.y, zOf(a));
export const dot3 = (a: Vec3, b: Vec3): number => a.x * b.x + a.y * b.y + zOf(a) * zOf(b);
export const cross3 = (a: Vec3, b: Vec3): Vec3 => ({
    x: a.y * zOf(b) - zOf(a) * b.y,
    y: zOf(a) * b.x - a.x * zOf(b),
    z: a.x * b.y - a.y * b.x,
});
export const norm3 = (a: Vec3): Vec3 => {
    const length = len3(a);
    return length < 1e-9 ? { x: 0, y: 0, z: 0 } : mul3(a, 1 / length);
};
export const dist3 = (a: Vec3, b: Vec3): number => len3(sub3(a, b));

export const rad = (degrees: number) => (degrees * Math.PI) / 180;

// Rodrigues: turn `point` about a unit `axis` through the origin.
export const rotateAxis = (point: Vec3, axis: Vec3, angle: number): Vec3 => {
    const cos = Math.cos(angle);
    const sin = Math.sin(angle);
    return add3(
        add3(mul3(point, cos), mul3(cross3(axis, point), sin)),
        mul3(axis, dot3(axis, point) * (1 - cos)),
    );
};


export const lerp3 = (a: Vec3, b: Vec3, t: number): Vec3 => ({
    x: a.x + (b.x - a.x) * t,
    y: a.y + (b.y - a.y) * t,
    z: zOf(a) + (zOf(b) - zOf(a)) * t,
});
