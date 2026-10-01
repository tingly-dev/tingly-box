// Saving a blob with the anchor-click dance: what a browser tab needs. The
// desktop bridge saves through a native dialog instead and uses this only as
// its fallback when that request fails.
export const saveFileViaAnchor = (blob: Blob, fileName: string): void => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = fileName;
    document.body.appendChild(link);
    link.click();
    link.remove();
    // Revoking synchronously can cancel the download in some browsers.
    setTimeout(() => URL.revokeObjectURL(url), 10_000);
};
