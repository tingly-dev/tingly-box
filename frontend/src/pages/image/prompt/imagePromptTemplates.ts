// The instructions the vision model gets on the Image to Prompt page
// (.design/image-to-prompt.md). They are model-facing, so English, and they
// are the user's to edit: the page starts from DEFAULT_INSTRUCTIONS and the
// starters below only fill the field, they are not a mode.
//
// The shape follows what captioners built for this job settled on (JoyCaption's
// "straightforward" and "Stable Diffusion prompt" types, Florence-2's detailed
// caption): lead with subject and medium, then a fixed order of visual facts,
// no guessing, no filler adjectives, visible text verbatim, the prompt only.

export const INSTRUCTION_STARTERS = {
    natural: `You write prompts for text-to-image models. Look at the image and write one prompt that would make an image model produce a picture like it.

- Start with the main subject and the medium (photo, oil painting, 3D render, anime illustration, vector art…).
- Then, in this order: what the subject looks like and is doing; the setting and background; composition and camera (shot type, angle, lens, depth of field); lighting; colour palette; style or artistic references.
- State concrete, visible facts. Do not guess at what you cannot see, and do not use empty praise such as "beautiful", "stunning" or "masterpiece".
- If the image contains legible text, quote it exactly.
- One paragraph of natural English sentences, about 60–120 words.
- Output only the prompt: no title, no preamble, no quotation marks around it.`,
    tags: `You write prompts for Stable Diffusion-style image models. Look at the image and write it as a comma-separated list of tags.

- Most important first: medium and subject, then appearance, pose and action, clothing, setting, composition and camera, lighting, colours, style.
- Short lowercase tags of one to four words each, about 20–40 tags.
- Only what is visible. No sentences, no empty praise tags.
- Output only the tag list.`,
    style: `You describe visual style so it can be reused on other subjects. Look at the image and describe only how it is made, not what it shows.

- Medium and technique, rendering and brushwork, line and texture, lighting, colour palette, composition habits, era or artistic references.
- Do not mention the subject, the people, the objects or the place.
- A comma-separated list of short phrases, about 15–30.
- Output only the list.`,
};

export type InstructionStarter = keyof typeof INSTRUCTION_STARTERS;
export const STARTERS = Object.keys(INSTRUCTION_STARTERS) as InstructionStarter[];

export const DEFAULT_INSTRUCTIONS = INSTRUCTION_STARTERS.natural;
export const isDefaultInstructions = (instructions: string) => instructions.trim() === DEFAULT_INSTRUCTIONS;

// The user turn that travels with the picture. The instructions carry the
// how; this only says which picture.
export const USER_REQUEST = 'Write the prompt for this image.';

// Models wrap answers they were told not to wrap: a code fence, a "Prompt:"
// label, a pair of quotes around the whole thing. Take those off so what
// lands in the card is the prompt itself.
export const cleanPromptReply = (reply: string): string => {
    let text = reply.trim();
    const fence = text.match(/^```[\w-]*\n([\s\S]*?)\n?```$/);
    if (fence) text = fence[1].trim();
    text = text.replace(/^(?:\*\*)?prompt(?:\*\*)?\s*[:：]\s*/i, '');
    const quoted = text.match(/^["“'](.*)["”']$/s);
    if (quoted && !/["“”]/.test(quoted[1])) text = quoted[1].trim();
    return text;
};

// Which starter the field still matches, so the menu can mark it and the page
// can tell an edited instruction from an untouched one.
export const matchingStarter = (instructions: string): InstructionStarter | null => {
    const text = instructions.trim();
    return STARTERS.find((key) => INSTRUCTION_STARTERS[key] === text) ?? null;
};
