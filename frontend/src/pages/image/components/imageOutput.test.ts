import { describe, it, expect } from 'vitest';
import {
    DEFAULT_OUTPUT_OPTIONS,
    FRAME_COUNT_RANGE,
    FRAME_DURATION_RANGE,
    buildGenerateParams,
    clampInt,
    describeOutput,
    isDefaultOutput,
    mimeForFormat,
    normalizeOutputOptions,
    outputRequestFields,
    resolveResultMime,
    sniffImageMime,
    thumbnailType,
} from './imageOutput';

const b64 = (bytes: number[]) => btoa(String.fromCharCode(...bytes));
const PNG = b64([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0]);
const JPEG = b64([0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 0x4a, 0x46, 0x49, 0x46]);
const GIF = b64([0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 1, 0, 1, 0]);
const WEBP = b64([0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50]);

describe('sniffImageMime', () => {
    it('recognises the four formats from the base64 header', () => {
        expect(sniffImageMime(PNG)).toBe('image/png');
        expect(sniffImageMime(JPEG)).toBe('image/jpeg');
        expect(sniffImageMime(GIF)).toBe('image/gif');
        expect(sniffImageMime(WEBP)).toBe('image/webp');
    });

    it('matches the documented base64 prefixes', () => {
        expect(PNG.startsWith('iVBOR')).toBe(true);
        expect(JPEG.startsWith('/9j/')).toBe(true);
        expect(GIF.startsWith('R0lGOD')).toBe(true);
        expect(WEBP.startsWith('UklGR')).toBe(true);
    });

    it('returns null for anything else, svg included', () => {
        expect(sniffImageMime('')).toBeNull();
        expect(sniffImageMime('aW1n')).toBeNull();
        expect(sniffImageMime(btoa('<svg xmlns="http://www.w3.org/2000/svg"/>'))).toBeNull();
    });

    it('ignores surrounding whitespace', () => {
        expect(sniffImageMime(`\n${GIF}\n`)).toBe('image/gif');
    });
});

describe('resolveResultMime', () => {
    it('trusts the response output_format over the bytes', () => {
        expect(resolveResultMime('gif', PNG)).toBe('image/gif');
        expect(resolveResultMime('jpeg', undefined)).toBe('image/jpeg');
    });

    it('sniffs the bytes when the response says nothing', () => {
        expect(resolveResultMime(undefined, GIF)).toBe('image/gif');
        expect(resolveResultMime(null, WEBP)).toBe('image/webp');
    });

    it('ignores an unknown format name (svg is never produced) and falls back to the bytes, then png', () => {
        expect(resolveResultMime('svg', JPEG)).toBe('image/jpeg');
        expect(resolveResultMime('svg', 'aW1n')).toBe('image/png');
        expect(resolveResultMime(undefined, undefined)).toBe('image/png');
    });
});

describe('mimeForFormat', () => {
    it('maps jpeg to image/jpeg and the rest to image/<name>', () => {
        expect(mimeForFormat('png')).toBe('image/png');
        expect(mimeForFormat('jpeg')).toBe('image/jpeg');
        expect(mimeForFormat('webp')).toBe('image/webp');
        expect(mimeForFormat('gif')).toBe('image/gif');
    });
});

describe('clampInt', () => {
    it('clamps, rounds and falls back on non-numbers', () => {
        expect(clampInt(100, 2, 16, 9)).toBe(16);
        expect(clampInt(-3, 2, 16, 9)).toBe(2);
        expect(clampInt(7.6, 2, 16, 9)).toBe(8);
        expect(clampInt(Number.NaN, 2, 16, 9)).toBe(9);
        expect(clampInt('12', 2, 16, 9)).toBe(12);
        expect(clampInt(undefined, 2, 16, 9)).toBe(9);
    });
});

describe('outputRequestFields', () => {
    it('sends nothing for the default, so the request is identical to before', () => {
        expect(outputRequestFields(DEFAULT_OUTPUT_OPTIONS)).toEqual({});
        expect(outputRequestFields(undefined)).toEqual({});
        expect(isDefaultOutput(DEFAULT_OUTPUT_OPTIONS)).toBe(true);
    });

    it('sends only output_format for jpeg and webp', () => {
        expect(outputRequestFields({ ...DEFAULT_OUTPUT_OPTIONS, format: 'jpeg', frameCount: 4 })).toEqual({ output_format: 'jpeg' });
        expect(outputRequestFields({ ...DEFAULT_OUTPUT_OPTIONS, format: 'webp' })).toEqual({ output_format: 'webp' });
    });

    it('adds frame_count and frame_duration_ms for gif', () => {
        expect(outputRequestFields({ format: 'gif', frameCount: 9, frameDurationMs: 200 })).toEqual({
            output_format: 'gif',
            frame_count: 9,
            frame_duration_ms: 200,
        });
        expect(outputRequestFields({ format: 'gif', frameCount: 12, frameDurationMs: 80 })).toEqual({
            output_format: 'gif',
            frame_count: 12,
            frame_duration_ms: 80,
        });
    });

    it('clamps out-of-range gif values into the documented ranges', () => {
        expect(outputRequestFields({ format: 'gif', frameCount: 99, frameDurationMs: 5 })).toMatchObject({
            frame_count: FRAME_COUNT_RANGE.max,
            frame_duration_ms: FRAME_DURATION_RANGE.min,
        });
        expect(outputRequestFields({ format: 'gif', frameCount: 1, frameDurationMs: 99999 })).toMatchObject({
            frame_count: FRAME_COUNT_RANGE.min,
            frame_duration_ms: FRAME_DURATION_RANGE.max,
        });
    });
});

describe('normalizeOutputOptions', () => {
    it('fills a partial or legacy value with the defaults', () => {
        expect(normalizeOutputOptions(undefined)).toEqual(DEFAULT_OUTPUT_OPTIONS);
        expect(normalizeOutputOptions({ format: 'gif' })).toEqual({ format: 'gif', frameCount: 9, frameDurationMs: 200 });
        expect(normalizeOutputOptions({ format: 'svg' as never })).toEqual(DEFAULT_OUTPUT_OPTIONS);
    });
});

describe('buildGenerateParams', () => {
    const base = { model: 'gpt-image-1', prompt: 'a cat', count: 2, size: '1024x1024', quality: 'auto' as const };

    it('is the plain request when the format is left at the default', () => {
        expect(buildGenerateParams({ ...base, output: DEFAULT_OUTPUT_OPTIONS })).toEqual({
            model: 'gpt-image-1',
            prompt: 'a cat',
            n: 2,
            size: '1024x1024',
            quality: 'auto',
        });
        expect(buildGenerateParams(base)).not.toHaveProperty('output_format');
    });

    it('carries the gif fields next to the OpenAI body', () => {
        expect(buildGenerateParams({ ...base, output: { format: 'gif', frameCount: 6, frameDurationMs: 120 } })).toEqual({
            model: 'gpt-image-1',
            prompt: 'a cat',
            n: 2,
            size: '1024x1024',
            quality: 'auto',
            output_format: 'gif',
            frame_count: 6,
            frame_duration_ms: 120,
        });
    });
});

describe('thumbnailType', () => {
    it('keeps a gif as a lossless still and re-encodes the rest as webp', () => {
        expect(thumbnailType('image/gif')).toBe('image/png');
        expect(thumbnailType('image/png')).toBe('image/webp');
        expect(thumbnailType('')).toBe('image/webp');
    });
});

describe('describeOutput', () => {
    it('is empty for the default and concrete otherwise', () => {
        expect(describeOutput(undefined)).toBe('');
        expect(describeOutput({ format: 'webp' })).toBe('webp');
        expect(describeOutput({ format: 'gif', frameCount: 6, frameDurationMs: 120 })).toBe('gif · 6 × 120 ms');
    });
});
