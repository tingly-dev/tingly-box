// Renders the expression reference face: a VRM avatar trimmed down to its
// face (frontend/scripts/trim-vrm-face.mjs), posed with the VRM standard
// expressions and photographed head-on.
//
// Only this module imports three-vrm, and only the expression dialog imports
// this module — lazily — so neither the model (≈1.4 MB, served from
// /assets like any static file) nor the loader is fetched until someone
// opens the dialog.
import * as THREE from 'three';
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader.js';
import { VRMLoaderPlugin, VRMUtils, type VRM } from '@pixiv/three-vrm';
import type { ExpressionState } from './expressionState';

export const FACE_MODEL_URL = `${import.meta.env.BASE_URL}assets/expression/face.vrm`;

// How far the eyes can turn, in degrees. VRM maps this onto each model's own
// eye range, so the extremes are "as far as this face can look".
const GAZE_YAW = 30;
const GAZE_PITCH = 20;

// White, like every other reference image: the face is the subject, and a
// coloured ground is one more thing the model could decide to copy.
const BACKGROUND = '#ffffff';

// Where the features sit on the head, as fractions of its height from the
// chin: the frame is centred a little under halfway up (the eyes of an anime
// face sit low) and spans a bit more than chin-to-brows.
const FEATURES_CENTER = 0.44;
const FEATURES_HALF_HEIGHT = 0.5;

interface Stage {
    vrm: VRM;
    renderer: THREE.WebGLRenderer;
    scene: THREE.Scene;
    camera: THREE.PerspectiveCamera;
}

let stagePromise: Promise<Stage> | null = null;

const buildStage = async (): Promise<Stage> => {
    const loader = new GLTFLoader().register((parser) => new VRMLoaderPlugin(parser));
    const gltf = await loader.loadAsync(FACE_MODEL_URL);
    const vrm = gltf.userData.vrm as VRM;
    VRMUtils.rotateVRM0(vrm);
    if (vrm.lookAt) vrm.lookAt.autoUpdate = false;

    const scene = new THREE.Scene();
    scene.background = new THREE.Color(BACKGROUND);
    // A soft key from the front and above, like a portrait: enough shading to
    // read the face's shape, never enough to make a shadow a feature.
    const key = new THREE.DirectionalLight(0xffffff, 2.2);
    key.position.set(0.5, 1, 2);
    scene.add(key, new THREE.AmbientLight(0xffffff, 0.8), vrm.scene);

    // Framed on the face mesh itself, in its resting pose — not on the head
    // bone, which sits at the base of the skull. And framed on the *features*:
    // brows to chin fill the picture and the bald crown is cropped off, both
    // because the expression is what is being shown and because a bare skull
    // is one more thing the model could decide to draw. Measured once: an
    // expression must never re-frame the picture.
    vrm.update(0);
    const bounds = new THREE.Box3().setFromObject(vrm.scene, true);
    const center = bounds.getCenter(new THREE.Vector3());
    const size = bounds.getSize(new THREE.Vector3());
    center.y = bounds.min.y + size.y * FEATURES_CENTER;
    const fov = 18;
    const camera = new THREE.PerspectiveCamera(fov, 1, 0.01, 20);
    const half = size.y * FEATURES_HALF_HEIGHT;
    const distance = half / Math.tan(THREE.MathUtils.degToRad(fov / 2));
    camera.position.set(center.x, center.y, center.z + distance);
    camera.lookAt(center);

    const renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    renderer.setPixelRatio(1);
    return { vrm, renderer, scene, camera };
};

// Loads once and shares: every preview, thumbnail and export uses the same
// avatar and the same WebGL context.
export const loadFaceStage = (): Promise<Stage> => {
    if (!stagePromise) {
        stagePromise = buildStage().catch((error) => {
            stagePromise = null; // let a later open retry
            throw error;
        });
    }
    return stagePromise;
};

const pose = ({ vrm }: Stage, state: ExpressionState) => {
    const expressions = vrm.expressionManager;
    if (expressions) {
        for (const name of Object.keys(expressions.expressionMap)) expressions.setValue(name, 0);
        for (const [name, value] of Object.entries(state.weights)) expressions.setValue(name, value ?? 0);
    }
    if (vrm.lookAt) {
        // Viewer's left/right: the avatar faces the camera, so its yaw is the
        // mirror of what the viewer calls "looking left".
        vrm.lookAt.yaw = -state.gaze.x * GAZE_YAW;
        vrm.lookAt.pitch = state.gaze.y * GAZE_PITCH;
    }
    vrm.update(0);
};

// Draws the face into `target` (resized to `size`). One shared WebGL canvas,
// copied out, so any number of previews can exist at once.
export const drawFace = (stage: Stage, state: ExpressionState, target: HTMLCanvasElement, size: number): void => {
    pose(stage, state);
    stage.renderer.setSize(size, size, false);
    stage.renderer.render(stage.scene, stage.camera);
    target.width = size;
    target.height = size;
    target.getContext('2d')?.drawImage(stage.renderer.domElement, 0, 0);
};

export const exportFace = async (stage: Stage, state: ExpressionState, size = 1024): Promise<{ file: File; previewUrl: string }> => {
    const canvas = document.createElement('canvas');
    drawFace(stage, state, canvas, size);
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
    if (!blob) throw new Error('could not encode the expression image');
    return {
        file: new File([blob], `expression-${Date.now()}.png`, { type: 'image/png' }),
        previewUrl: canvas.toDataURL('image/png'),
    };
};
