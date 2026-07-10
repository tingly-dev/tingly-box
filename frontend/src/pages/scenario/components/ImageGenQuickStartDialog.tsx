import { useState } from 'react';
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
    Typography,
} from '@mui/material';
import { Close } from '@/components/icons';
import CodeBlock from '@/components/CodeBlock';

interface ImageGenQuickStartDialogProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    model?: string;
    onCopy?: (text: string, label: string) => void;
}

type Lang = 'python' | 'typescript' | 'curl';

const TABS: { value: Lang; label: string; filename: string }[] = [
    { value: 'python', label: 'Python', filename: 'imagegen.py' },
    { value: 'typescript', label: 'TypeScript', filename: 'imagegen.ts' },
    { value: 'curl', label: 'curl', filename: 'imagegen.sh' },
];

const buildSnippet = (lang: Lang, baseUrl: string, model: string): string => {
    const endpoint = `${baseUrl}/tingly/imagegen/v1`;
    const prompt = 'A cozy cabin in a snowy forest at dusk, cinematic lighting';
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
    prompt="${prompt}",
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
  prompt: "${prompt}",
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
curl ${endpoint}/images/generations \
  -H "Authorization: Bearer <TINGLY_MODEL_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "${model}",
    "prompt": "${prompt}",
    "size": "1024x1024",
    "quality": "auto",
    "n": 1
  }' \
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
    const [tab, setTab] = useState<Lang>('python');
    const active = TABS.find((item) => item.value === tab)!;
    const code = buildSnippet(tab, baseUrl, model);

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="md"
            fullWidth
            PaperProps={{ sx: { borderRadius: 3 } }}
        >
            <DialogTitle sx={{ pb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography component="span" variant="h6" fontWeight={600}>Image Generation Quick Start</Typography>
                <IconButton onClick={onClose} size="small" aria-label="Close quick start">
                    <Close fontSize="small" />
                </IconButton>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    Call the image generation endpoint, then decode the base64 response into an image file.
                    The model token is available from GET /api/v1/token.
                </Typography>
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
                        filename={active.filename}
                        onCopy={onCopy ? (content) => onCopy(content, active.filename) : undefined}
                        maxHeight={480}
                        wrap={false}
                    />
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
                <Button onClick={onClose} variant="contained">Done</Button>
            </DialogActions>
        </Dialog>
    );
};

export default ImageGenQuickStartDialog;
