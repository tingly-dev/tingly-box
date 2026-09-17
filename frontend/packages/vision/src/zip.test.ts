import { describe, expect, it } from 'vitest';
import { createZip, crc32 } from './zip';

const readU16 = (data: Uint8Array, offset: number) => data[offset] | (data[offset + 1] << 8);
const readU32 = (data: Uint8Array, offset: number) =>
    (data[offset] | (data[offset + 1] << 8) | (data[offset + 2] << 16) | (data[offset + 3] << 24)) >>> 0;

const bytes = (text: string) => new TextEncoder().encode(text);

describe('crc32', () => {
    // The canonical CRC-32 check value from the standard's own test vector.
    it('matches the known value for "123456789"', () => {
        expect(crc32(bytes('123456789'))).toBe(0xCBF43926);
    });

    it('is 0 for empty input', () => {
        expect(crc32(new Uint8Array())).toBe(0);
    });
});

describe('createZip', () => {
    const entries = [
        { name: 'sticker-1.png', data: bytes('first') },
        { name: '贴纸-2.png', data: bytes('second payload') },
    ];
    const zip = createZip(entries, new Date(2024, 4, 17, 10, 30, 20));

    it('starts with a local file header and ends with the EOCD record', () => {
        expect(readU32(zip, 0)).toBe(0x04034B50);
        const eocd = zip.length - 22;
        expect(readU32(zip, eocd)).toBe(0x06054B50);
    });

    it('records every entry once in the central directory', () => {
        const eocd = zip.length - 22;
        expect(readU16(zip, eocd + 8)).toBe(entries.length);
        expect(readU16(zip, eocd + 10)).toBe(entries.length);
    });

    it('points the EOCD at a central directory of the stated size', () => {
        const eocd = zip.length - 22;
        const centralSize = readU32(zip, eocd + 12);
        const centralOffset = readU32(zip, eocd + 16);
        expect(centralOffset + centralSize).toBe(eocd);
        expect(readU32(zip, centralOffset)).toBe(0x02014B50);
    });

    it('stores payloads uncompressed, with matching sizes and checksum', () => {
        expect(readU16(zip, 8)).toBe(0); // method: stored
        expect(readU32(zip, 14)).toBe(crc32(entries[0].data));
        expect(readU32(zip, 18)).toBe(entries[0].data.length);
        expect(readU32(zip, 22)).toBe(entries[0].data.length);
        const nameLength = readU16(zip, 26);
        const payload = zip.slice(30 + nameLength, 30 + nameLength + entries[0].data.length);
        expect(new TextDecoder().decode(payload)).toBe('first');
    });

    it('flags names as UTF-8 so non-ASCII filenames survive', () => {
        expect(readU16(zip, 6) & 0x0800).toBe(0x0800);
        const nameLength = readU16(zip, 26);
        expect(new TextDecoder().decode(zip.slice(30, 30 + nameLength))).toBe('sticker-1.png');
    });

    it('produces a valid, empty archive for no entries', () => {
        const empty = createZip([]);
        expect(empty.length).toBe(22);
        expect(readU32(empty, 0)).toBe(0x06054B50);
    });
});
