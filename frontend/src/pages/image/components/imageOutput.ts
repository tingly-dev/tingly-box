// What the Image Playground asks the gateway to produce (output format and, for
// GIF, its frames) and what it does with the bytes that come back. All pure, so
// the contract with POST /tingly/imagegen/v1/images/generations is under test
// (imageOutput.test.ts). See .design/image-output-formats.md.

import type { Quality } from './ImageGenPlayground.types';

export type OutputFormat = 'png' | 'jpeg' | 'webp' | 'gif';

export const OUTPUT_FORMATS: readonly OutputFormat[] = ['png', 'jpeg', 'webp', 'gif'];
export const DEFAULT_OUTPUT_FORMAT: OutputFormat = 'png';

export const FRAME_COUNT_RANGE = { min: 2, max: 16, default: 9 } as const;
export const FRAME_DURATION_RANGE = { min: 20, max: 2000, default: 200 } as const;

export interface OutputOptions {
    format: OutputFormat;
    // Only meaningful for gif; kept for the other formats so switching back
    // and forth does not lose what was typed.
    frameCount: number;
    frameDurationMs: number;
}

export const DEFAULT_OUTPUT_OPTIONS: OutputOptions = {
    format: DEFAULT_OUTPUT_FORMAT,
    frameCount: FRAME_COUNT_RANGE.default,
    frameDurationMs: FRAME_DURATION_RANGE.default,
};

const isOutputFormat = (value: unknown): value is OutputFormat => (
    typeof value === 'string' && (OUTPUT_FORMATS as readonly string[]).includes(value)
);

export const mimeForFormat = (format: OutputFormat): string => `image/${format}`;

export const clampInt = (value: unknown, min: number, max: number, fallback: number): number => {
    const number = typeof value === 'string' && value.trim() === '' ? Number.NaN : Number(value);
    if (!Number.isFinite(number)) return fallback;
    return Math.min(max, Math.max(min, Math.round(number)));
};

/** A stored or partial value as a complete one (old runs and profiles have none). */
export const normalizeOutputOptions = (value?: Partial<OutputOptions> | null): OutputOptions => ({
    format: isOutputFormat(value?.format) ? value.format : DEFAULT_OUTPUT_FORMAT,
    frameCount: clampInt(value?.frameCount, FRAME_COUNT_RANGE.min, FRAME_COUNT_RANGE.max, FRAME_COUNT_RANGE.default),
    frameDurationMs: clampInt(
        value?.frameDurationMs,
        FRAME_DURATION_RANGE.min,
        FRAME_DURATION_RANGE.max,
        FRAME_DURATION_RANGE.default,
    ),
});

export const isDefaultOutput = (value?: Partial<OutputOptions> | null): boolean => (
    normalizeOutputOptions(value).format === DEFAULT_OUTPUT_FORMAT
);

/**
 * The extra JSON keys for the request: none for the default (PNG), so a request
 * the user did not touch is byte-for-byte what it was before; `output_format`
 * for the other formats; plus `frame_count` / `frame_duration_ms` for gif.
 */
export const outputRequestFields = (
    value?: Partial<OutputOptions> | null,
): { output_format?: OutputFormat; frame_count?: number; frame_duration_ms?: number } => {
    const output = normalizeOutputOptions(value);
    if (output.format === DEFAULT_OUTPUT_FORMAT) return {};
    if (output.format !== 'gif') return { output_format: output.format };
    return {
        output_format: 'gif',
        frame_count: output.frameCount,
        frame_duration_ms: output.frameDurationMs,
    };
};

export interface GenerateParamsInput {
    model: string;
    prompt: string;
    count: number;
    size: string;
    quality: Quality;
    output?: Partial<OutputOptions> | null;
}

/** The body of images/generations: the OpenAI image fields plus the output fields. */
export const buildGenerateParams = ({ model, prompt, count, size, quality, output }: GenerateParamsInput) => ({
    model,
    prompt,
    n: count,
    size,
    quality,
    ...outputRequestFields(output),
});

// The bytes are the truth about what came back: the gateway reports it in the
// response's `output_format`, and when a response (or an older gateway) does
// not, the header of the data says it. Only these four are ever produced.
const SNIFF_PREFIXES: ReadonlyArray<readonly [string, string]> = [
    ['iVBOR', 'image/png'],
    ['/9j/', 'image/jpeg'],
    ['R0lGOD', 'image/gif'],
    ['UklGR', 'image/webp'],
];

export const sniffImageMime = (b64: string | undefined | null): string | null => {
    if (!b64) return null;
    const head = b64.trimStart();
    return SNIFF_PREFIXES.find(([prefix]) => head.startsWith(prefix))?.[1] ?? null;
};

/** Response `output_format`, else the bytes, else PNG. */
export const resolveResultMime = (
    responseFormat: unknown,
    b64: string | undefined | null,
): string => (isOutputFormat(responseFormat)
    ? mimeForFormat(responseFormat)
    : sniffImageMime(b64) ?? 'image/png');

/**
 * What a thumbnail is encoded as. A GIF is shown as a lossless still of its
 * first frame (re-encoding it as lossy webp is what flattens it); the animated
 * original lives in the lightbox.
 */
export const thumbnailType = (mime: string): string => (mime === 'image/gif' ? 'image/png' : 'image/webp');

/** The run line's short form: `webp`, `gif · 9 × 200 ms`; empty for the default. */
export const describeOutput = (value?: Partial<OutputOptions> | null): string => {
    const output = normalizeOutputOptions(value);
    if (output.format === DEFAULT_OUTPUT_FORMAT) return '';
    return output.format === 'gif'
        ? `gif · ${output.frameCount} × ${output.frameDurationMs} ms`
        : output.format;
};
