// "Get a prompt from this image" elsewhere hands the picture to the Image to
// Prompt page here rather than through router state, which would clone a
// multi-MB data URL into session history. Its own module so the lightbox can
// use it without loading the page's run store.
let handedPicture: { src: string; name: string } | null = null;

export const handOffPicture = (src: string, name: string) => { handedPicture = { src, name }; };

// Once: a later visit to the page starts empty.
export const takeHandedPicture = () => {
    const picture = handedPicture;
    handedPicture = null;
    return picture;
};
