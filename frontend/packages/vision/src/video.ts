// The sliced tiles, handed over as a video instead of a GIF.
//
// A GIF is what the browser can build alone, but it is also what chat apps
// and social platforms re-compress, cap by size, or refuse outright. An H.264
// MP4 is the one format all of them forward as-is. Encoding it is delegated to
// the browser's own WebCodecs encoder, and boxing it into MP4 (or WebM, when
// no H.264 encoder exists) to `mediabunny` — an MP4 muxer is not the small
// half of any spec, so unlike `gif.ts` / `zip.ts` this is not hand-rolled.
//
// The `mediabunny` import stays dynamic: the library is only pulled into the
// bundle the first time someone actually asks for a video.

import type { GifFrame } from './gif';

export interface VideoTarget {
    container: 'mp4' | 'webm';
    codec: 'avc' | 'vp9' | 'vp8';
    extension: 'mp4' | 'webm';
}

export interface VideoOptions {
    width: number;
    height: number;
    frames: GifFrame[];
    /** Per-frame duration in milliseconds. */
    delayMs: number;
    /** How many times the sequence plays; `planVideoLoops` picks a default. */
    loops: number;
    /**
     * Video has no alpha, so anything a matte left transparent lands on this
     * colour. CSS colour string.
     */
    background: string;
    target: VideoTarget;
}

// Sub-second clips get dropped or refused by the platforms people forward to,
// and a sticker loop is often well under a second — so the sequence repeats
// until the clip is at least this long.
export const MIN_VIDEO_MS = 3000;
export const MAX_VIDEO_LOOPS = 60;

/** The smallest number of loops that gets one sequence past `MIN_VIDEO_MS`. */
export const planVideoLoops = (frameCount: number, delayMs: number): number => {
    const once = frameCount * delayMs;
    if (once <= 0) return 1;
    return Math.min(MAX_VIDEO_LOOPS, Math.max(1, Math.ceil(MIN_VIDEO_MS / once)));
};

/**
 * 4:2:0 chroma subsampling needs even dimensions; an odd frame is padded by a
 * pixel rather than cropped, so nothing the user selected is lost.
 */
export const evenSize = (size: { width: number; height: number }): { width: number; height: number } => ({
    width: size.width + (size.width % 2),
    height: size.height + (size.height % 2),
});

export const videoFileName = (stem: string, count: number, target: VideoTarget): string => (
    `${stem}-${count}.${target.extension}`
);

/**
 * Which container / codec this browser can produce, MP4 + H.264 first because
 * it is the one that forwards everywhere. `null` when the browser has no
 * WebCodecs at all — the button then says so instead of failing on click.
 */
export const probeVideoTarget = async (
    size: { width: number; height: number },
): Promise<VideoTarget | null> => {
    if (typeof VideoEncoder === 'undefined') return null;
    const { getFirstEncodableVideoCodec } = await import('mediabunny');
    const even = evenSize(size);
    if (await getFirstEncodableVideoCodec(['avc'], even)) {
        return { container: 'mp4', codec: 'avc', extension: 'mp4' };
    }
    const fallback = await getFirstEncodableVideoCodec(['vp9', 'vp8'], even);
    if (fallback === 'vp9' || fallback === 'vp8') {
        return { container: 'webm', codec: fallback, extension: 'webm' };
    }
    return null;
};

const context2d = (canvas: HTMLCanvasElement): CanvasRenderingContext2D => {
    const context = canvas.getContext('2d');
    if (!context) throw new Error('canvas 2d context unavailable');
    return context;
};

export const encodeVideo = async (options: VideoOptions): Promise<Blob> => {
    const { frames, delayMs, loops, background, target } = options;
    if (frames.length === 0) throw new Error('video needs at least one frame');
    const {
        BufferTarget, CanvasSource, Mp4OutputFormat, Output, QUALITY_HIGH, WebMOutputFormat,
    } = await import('mediabunny');

    const size = evenSize({ width: options.width, height: options.height });

    // Frames are RGBA buffers; `putImageData` would carry their alpha straight
    // through, so each one is composited over the background on a second
    // canvas before the encoder sees it.
    const frameCanvas = document.createElement('canvas');
    frameCanvas.width = options.width;
    frameCanvas.height = options.height;
    const frameContext = context2d(frameCanvas);

    const canvas = document.createElement('canvas');
    canvas.width = size.width;
    canvas.height = size.height;
    const context = context2d(canvas);

    const output = new Output({
        // `in-memory` writes `moov` ahead of `mdat`, which is what lets a
        // platform start playing (or accept) the file before it has all of it.
        format: target.container === 'mp4'
            ? new Mp4OutputFormat({ fastStart: 'in-memory' })
            : new WebMOutputFormat(),
        target: new BufferTarget(),
    });
    const source = new CanvasSource(canvas, {
        codec: target.codec,
        quality: QUALITY_HIGH,
        // Every frame a key frame would bloat the file; one per loop keeps
        // seeking snappy without that.
        keyFrameInterval: Math.max(1, (frames.length * delayMs) / 1000),
    });
    output.addVideoTrack(source, { frameRate: 1000 / delayMs });
    await output.start();

    const delaySeconds = delayMs / 1000;
    let index = 0;
    for (let loop = 0; loop < loops; loop += 1) {
        for (const frame of frames) {
            const imageData = new ImageData(new Uint8ClampedArray(frame.data), options.width, options.height);
            frameContext.putImageData(imageData, 0, 0);
            context.fillStyle = background;
            context.fillRect(0, 0, size.width, size.height);
            context.drawImage(frameCanvas, 0, 0);
            await source.add(index * delaySeconds, delaySeconds);
            index += 1;
        }
    }
    await output.finalize();

    const buffer = (output.target as InstanceType<typeof BufferTarget>).buffer;
    if (!buffer) throw new Error('encoder produced no output');
    return new Blob([buffer], { type: target.container === 'mp4' ? 'video/mp4' : 'video/webm' });
};
