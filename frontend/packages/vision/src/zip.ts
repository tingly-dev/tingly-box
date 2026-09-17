// Minimal ZIP writer (store method, no compression).
//
// The playground packs already-compressed PNGs, so deflating them buys
// essentially nothing — which is why this is ~100 lines of local code instead
// of a compression dependency. Everything here is the "stored" (method 0)
// subset of APPNOTE.TXT: a local file header per entry, a central directory,
// and an end-of-central-directory record.

export interface ZipEntry {
    name: string;
    data: Uint8Array;
}

const CRC_TABLE = (() => {
    const table = new Uint32Array(256);
    for (let i = 0; i < 256; i += 1) {
        let value = i;
        for (let bit = 0; bit < 8; bit += 1) {
            value = value & 1 ? 0xEDB88320 ^ (value >>> 1) : value >>> 1;
        }
        table[i] = value >>> 0;
    }
    return table;
})();

export const crc32 = (data: Uint8Array): number => {
    let crc = 0xFFFFFFFF;
    for (let i = 0; i < data.length; i += 1) {
        crc = CRC_TABLE[(crc ^ data[i]) & 0xFF] ^ (crc >>> 8);
    }
    return (crc ^ 0xFFFFFFFF) >>> 0;
};

// MS-DOS packed date/time, the only timestamp format a stored ZIP carries.
// Seconds have 2s granularity by design (5 bits).
const dosDateTime = (date: Date): { time: number; date: number } => ({
    time: (date.getHours() << 11) | (date.getMinutes() << 5) | (Math.floor(date.getSeconds() / 2)),
    date: ((Math.max(date.getFullYear(), 1980) - 1980) << 9) | ((date.getMonth() + 1) << 5) | date.getDate(),
});

class ByteWriter {
    private chunks: Uint8Array[] = [];
    length = 0;

    push(bytes: Uint8Array) {
        this.chunks.push(bytes);
        this.length += bytes.length;
    }

    u16(value: number) {
        this.push(new Uint8Array([value & 0xFF, (value >>> 8) & 0xFF]));
    }

    u32(value: number) {
        this.push(new Uint8Array([value & 0xFF, (value >>> 8) & 0xFF, (value >>> 16) & 0xFF, (value >>> 24) & 0xFF]));
    }

    concat(): Uint8Array {
        const out = new Uint8Array(this.length);
        let offset = 0;
        for (const chunk of this.chunks) {
            out.set(chunk, offset);
            offset += chunk.length;
        }
        return out;
    }
}

// Bit 11 of the general-purpose flags declares the filename is UTF-8, which is
// what every modern unarchiver reads; without it non-ASCII names decode as
// CP437 garbage.
const FLAG_UTF8 = 0x0800;

export const createZip = (entries: ZipEntry[], now: Date = new Date()): Uint8Array => {
    const encoder = new TextEncoder();
    const { time, date } = dosDateTime(now);
    const body = new ByteWriter();
    const central = new ByteWriter();

    for (const entry of entries) {
        const name = encoder.encode(entry.name);
        const checksum = crc32(entry.data);
        const offset = body.length;

        body.u32(0x04034B50);       // local file header signature
        body.u16(20);               // version needed to extract
        body.u16(FLAG_UTF8);
        body.u16(0);                // method: stored
        body.u16(time);
        body.u16(date);
        body.u32(checksum);
        body.u32(entry.data.length); // compressed size == uncompressed size
        body.u32(entry.data.length);
        body.u16(name.length);
        body.u16(0);                // extra field length
        body.push(name);
        body.push(entry.data);

        central.u32(0x02014B50);    // central directory header signature
        central.u16(20);            // version made by
        central.u16(20);            // version needed to extract
        central.u16(FLAG_UTF8);
        central.u16(0);
        central.u16(time);
        central.u16(date);
        central.u32(checksum);
        central.u32(entry.data.length);
        central.u32(entry.data.length);
        central.u16(name.length);
        central.u16(0);             // extra field length
        central.u16(0);             // file comment length
        central.u16(0);             // disk number start
        central.u16(0);             // internal attributes
        central.u32(0);             // external attributes
        central.u32(offset);
        central.push(name);
    }

    const centralOffset = body.length;
    const centralBytes = central.concat();
    body.push(centralBytes);
    body.u32(0x06054B50);           // end of central directory signature
    body.u16(0);                    // this disk number
    body.u16(0);                    // disk with the central directory
    body.u16(entries.length);
    body.u16(entries.length);
    body.u32(centralBytes.length);
    body.u32(centralOffset);
    body.u16(0);                    // comment length

    return body.concat();
};

export const createZipBlob = (entries: ZipEntry[], now?: Date): Blob =>
    new Blob([createZip(entries, now) as unknown as BlobPart], { type: 'application/zip' });
