// Turning files and URLs into the one image representation the Image pages
// use everywhere — a base64 data URL — plus the pixel size to caption it
// with. Shared by the playground (references, imports) and Assets.

/** Reads a File (or any Blob) into a data URL (`data:image/png;base64,...`). */
export const fileToDataUrl = (file: Blob): Promise<string> => new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
});

// Decodes an image just far enough to learn its pixel size. Failure is not
// worth surfacing — the caption simply drops the dimensions.
export const readImageSize = (src: string): Promise<{ width: number; height: number } | null> => new Promise((resolve) => {
    const image = new Image();
    image.onload = () => resolve({ width: image.naturalWidth, height: image.naturalHeight });
    image.onerror = () => resolve(null);
    image.src = src;
});
