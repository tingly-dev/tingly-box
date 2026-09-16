// Runs MediaPipe's pose landmarker on a photo, in the browser, and hands back
// landmarks in the shape `@tingly/mannequin` retargets from.
//
// Kept out of the mannequin package on purpose: the package knows nothing of
// MediaPipe or of where its files come from, and this adapter is the only
// place either shows up. The runtime is loaded lazily — it and its wasm are
// eighteen megabytes nobody pays for until they press the button.
import type { Landmark } from '@tingly/mannequin';

export interface DetectedPose {
    landmarks: Landmark[];
    // The estimator's own confidence that this is a person, 0..1.
    presence: number;
}

export interface Detection {
    poses: DetectedPose[];
    frame: { width: number; height: number };
}

type Landmarker = import('@mediapipe/tasks-vision').PoseLandmarker;

// How many people one photo may yield. A group photo becoming a group of
// figures is the cheapest surprise on this path, and the canvas already holds
// several; but past a few the result is a crowd nobody asked for.
export const MAX_POSES = 3;

let instance: Promise<Landmarker> | null = null;
let instanceBase: string | null = null;

// Wasm SIMD is the one thing the runtime we ship cannot do without.
const supportsSimd = (): boolean => {
    try {
        // The smallest valid module that uses a SIMD instruction.
        return WebAssembly.validate(new Uint8Array([
            0, 97, 115, 109, 1, 0, 0, 0, 1, 5, 1, 96, 0, 1, 123, 3, 2, 1, 0, 10, 10, 1, 8, 0, 65, 0, 253, 15, 253, 98, 11,
        ]));
    } catch {
        return false;
    }
};

export class PoseEstimatorUnsupported extends Error {}

// One landmarker per file base, built on first use and kept: creating it is
// the slow part (a second or two), detecting is not.
const landmarkerFor = (base: string): Promise<Landmarker> => {
    if (instance && instanceBase === base) return instance;
    instanceBase = base;
    instance = (async () => {
        if (!supportsSimd()) throw new PoseEstimatorUnsupported('wasm simd');
        const { FilesetResolver, PoseLandmarker } = await import('@mediapipe/tasks-vision');
        const vision = await FilesetResolver.forVisionTasks(base.replace(/\/$/, ''));
        const options = (delegate: 'GPU' | 'CPU') => ({
            baseOptions: { modelAssetPath: `${base}pose_landmarker_lite.task`, delegate },
            runningMode: 'IMAGE' as const,
            numPoses: MAX_POSES,
        });
        try {
            return await PoseLandmarker.createFromOptions(vision, options('GPU'));
        } catch {
            // No usable WebGL for the model (headless, a locked-down webview):
            // the CPU path is slower and gives the same landmarks.
            return PoseLandmarker.createFromOptions(vision, options('CPU'));
        }
    })();
    instance.catch(() => { instance = null; });
    return instance;
};

// `base` is the URL the gateway serves the runtime and model under, with a
// trailing slash — the same files whatever machine the UI is opened from.
export const detectPoses = async (image: HTMLImageElement, base: string): Promise<Detection> => {
    const landmarker = await landmarkerFor(base);
    const result = landmarker.detect(image);
    const poses: DetectedPose[] = result.landmarks.map((points) => {
        const landmarks = points.map((p) => ({
            x: p.x,
            y: p.y,
            z: p.z,
            visibility: p.visibility ?? 1,
        }));
        const seen = landmarks.map((p) => p.visibility);
        return {
            landmarks,
            presence: seen.length > 0 ? seen.reduce((sum, v) => sum + v, 0) / seen.length : 0,
        };
    });
    return { poses, frame: { width: image.naturalWidth, height: image.naturalHeight } };
};

// Decodes a picked file into an image the landmarker can read.
export const loadImageFile = (file: File): Promise<HTMLImageElement> => new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const image = new Image();
    image.onload = () => { URL.revokeObjectURL(url); resolve(image); };
    image.onerror = () => { URL.revokeObjectURL(url); reject(new Error('not an image')); };
    image.src = url;
});
