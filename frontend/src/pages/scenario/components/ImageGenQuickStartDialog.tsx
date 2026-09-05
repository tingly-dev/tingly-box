import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Tab,
    Tabs,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
} from '@mui/material';
import { AutoAwesome, Brush, Close } from '@/components/icons';
import CodeBlock from '@/components/CodeBlock';

interface ImageGenQuickStartDialogProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    model?: string;
    onCopy?: (text: string, label: string) => void;
}

type Lang = 'python' | 'typescript' | 'curl';
type Operation = 'generate' | 'edit';

const TABS: { value: Lang; label: string }[] = [
    { value: 'python', label: 'Python' },
    { value: 'typescript', label: 'TypeScript' },
    { value: 'curl', label: 'curl' },
];

const FILENAMES: Record<Operation, Record<Lang, string>> = {
    generate: { python: 'imagegen.py', typescript: 'imagegen.ts', curl: 'imagegen.sh' },
    edit: { python: 'imageedit.py', typescript: 'imageedit.ts', curl: 'imageedit.sh' },
};

const GENERATE_PROMPT = 'A cozy cabin in a snowy forest at dusk, cinematic lighting';
const EDIT_PROMPT = 'Add a red knit hat on the subject, keep everything else unchanged';

const buildSnippet = (op: Operation, lang: Lang, baseUrl: string, model: string): string => {
    const endpoint = `${baseUrl}/tingly/imagegen/v1`;

    if (op === 'edit') {
        switch (lang) {
            case 'python':
                return `# pip install openai
import base64
from openai import OpenAI

client = OpenAI(
    base_url="${endpoint}",
    api_key="<TINGLY_MODEL_TOKEN>",  # GET /api/v1/token
)

resp = client.images.edit(
    model="${model}",
    image=open("input.png", "rb"),
    prompt="${EDIT_PROMPT}",
    size="1024x1024",
    quality="auto",
)

image_b64 = resp.data[0].b64_json
with open("output.png", "wb") as f:
    f.write(base64.b64decode(image_b64))
print("Saved output.png")
`;
            case 'typescript':
                return `// npm i openai
import { createReadStream, writeFileSync } from "node:fs";
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${endpoint}",
  apiKey: "<TINGLY_MODEL_TOKEN>", // GET /api/v1/token
});

const resp = await client.images.edit({
  model: "${model}",
  image: createReadStream("input.png"),
  prompt: "${EDIT_PROMPT}",
  size: "1024x1024",
  quality: "auto",
});

const imageB64 = resp.data[0].b64_json!;
writeFileSync("output.png", Buffer.from(imageB64, "base64"));
console.log("Saved output.png");
`;
            case 'curl':
                return `# requires jq; decodes the base64 payload into output.png
curl ${endpoint}/images/edits \\
  -H "Authorization: Bearer <TINGLY_MODEL_TOKEN>" \\
  -F "model=${model}" \\
  -F "image=@input.png" \\
  -F "prompt=${EDIT_PROMPT}" \\
  -F "size=1024x1024" \\
  -F "quality=auto" \\
  | jq -r '.data[0].b64_json' | base64 --decode > output.png
`;
        }
    }

    switch (lang) {
        case 'python':
            return `# pip install openai
import base64
from openai import OpenAI

client = OpenAI(
    base_url="${endpoint}",
    api_key="<TINGLY_MODEL_TOKEN>",  # GET /api/v1/token
)

resp = client.images.generate(
    model="${model}",
    prompt="${GENERATE_PROMPT}",
    size="1024x1024",
    quality="auto",
    n=1,
)

image_b64 = resp.data[0].b64_json
with open("output.png", "wb") as f:
    f.write(base64.b64decode(image_b64))
print("Saved output.png")
`;
        case 'typescript':
            return `// npm i openai
import { writeFileSync } from "node:fs";
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${endpoint}",
  apiKey: "<TINGLY_MODEL_TOKEN>", // GET /api/v1/token
});

const resp = await client.images.generate({
  model: "${model}",
  prompt: "${GENERATE_PROMPT}",
  size: "1024x1024",
  quality: "auto",
  n: 1,
});

const imageB64 = resp.data[0].b64_json!;
writeFileSync("output.png", Buffer.from(imageB64, "base64"));
console.log("Saved output.png");
`;
        case 'curl':
            return `# requires jq; decodes the base64 payload into output.png
curl ${endpoint}/images/generations \\
  -H "Authorization: Bearer <TINGLY_MODEL_TOKEN>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "prompt": "${GENERATE_PROMPT}",
    "size": "1024x1024",
    "quality": "auto",
    "n": 1
  }' \\
  | jq -r '.data[0].b64_json' | base64 --decode > output.png
`;
    }
};

const ImageGenQuickStartDialog: React.FC<ImageGenQuickStartDialogProps> = ({
    open,
    onClose,
    baseUrl,
    model = 'gpt-image-1',
    onCopy,
}) => {
    const { t } = useTranslation();
    const [operation, setOperation] = useState<Operation>('generate');
    const [tab, setTab] = useState<Lang>('python');
    const filename = FILENAMES[operation][tab];
    const code = buildSnippet(operation, tab, baseUrl, model);

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="md"
            fullWidth
            slotProps={{
                paper: { sx: { borderRadius: 3 } }
            }}
        >
            <DialogTitle sx={{ pb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography component="span" variant="h6" sx={{
                    fontWeight: 600
                }}>{t('imageGenQuickStart.title')}</Typography>
                <IconButton onClick={onClose} size="small" aria-label={t('imageGenQuickStart.closeAriaLabel')}>
                    <Close fontSize="small" />
                </IconButton>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                <Typography
                    variant="body2"
                    sx={{
                        color: "text.secondary",
                        mb: 2
                    }}>
                    {t('imageGenQuickStart.description')}
                </Typography>
                <ToggleButtonGroup
                    value={operation}
                    exclusive
                    size="small"
                    onChange={(_, next: Operation | null) => { if (next) setOperation(next); }}
                    sx={{ mb: 1.5 }}
                >
                    <ToggleButton value="generate">
                        <AutoAwesome fontSize="small" sx={{ mr: 0.75 }} />
                        {t('image-playground.modeGenerate', { defaultValue: 'Generate' })}
                    </ToggleButton>
                    <ToggleButton value="edit">
                        <Brush fontSize="small" sx={{ mr: 0.75 }} />
                        {t('image-playground.modeEdit', { defaultValue: 'Edit' })}
                    </ToggleButton>
                </ToggleButtonGroup>
                <Tabs
                    value={tab}
                    onChange={(_, value: Lang) => setTab(value)}
                    sx={{ minHeight: 36, mb: 1, '& .MuiTabs-indicator': { height: 3 } }}
                >
                    {TABS.map((item) => (
                        <Tab
                            key={item.value}
                            value={item.value}
                            label={item.label}
                            sx={{ minHeight: 36, py: 0.5 }}
                        />
                    ))}
                </Tabs>
                <Box>
                    <CodeBlock
                        code={code}
                        language={tab === 'curl' ? 'bash' : tab}
                        filename={filename}
                        onCopy={onCopy ? (content) => onCopy(content, filename) : undefined}
                        maxHeight={480}
                        wrap={false}
                    />
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
                <Button onClick={onClose} variant="contained">{t('common.done')}</Button>
            </DialogActions>
        </Dialog>
    );
};

export default ImageGenQuickStartDialog;
