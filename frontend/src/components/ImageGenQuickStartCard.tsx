import { useState } from 'react';
import { Box, Collapse, IconButton, Tab, Tabs } from '@mui/material';
import { ExpandLess, ExpandMore } from '@mui/icons-material';
import UnifiedCard from '@/components/UnifiedCard';
import CodeBlock from '@/components/CodeBlock';

interface ImageGenQuickStartCardProps {
    baseUrl: string;
    /** Default model name shown in snippets. */
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

# Decode the base64 payload and write it to a file
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

// Decode the base64 payload and write it to a file
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
    "prompt": "${prompt}",
    "size": "1024x1024",
    "quality": "auto",
    "n": 1
  }' \\
  | jq -r '.data[0].b64_json' | base64 --decode > output.png
`;
    }
};

const ImageGenQuickStartCard: React.FC<ImageGenQuickStartCardProps> = ({
    baseUrl,
    model = 'gpt-image-1',
    onCopy,
}) => {
    const [tab, setTab] = useState<Lang>('python');
    const [expanded, setExpanded] = useState(true);
    const active = TABS.find((t) => t.value === tab)!;
    const code = buildSnippet(tab, baseUrl, model);

    return (
        <UnifiedCard
            size="full"
            title="Quick Start"
            subtitle="Call the image generation endpoint, then decode the base64 response into an image file. Token comes from GET /api/v1/token."
            rightAction={
                <IconButton size="small" onClick={() => setExpanded((v) => !v)}>
                    {expanded ? <ExpandLess fontSize="small" /> : <ExpandMore fontSize="small" />}
                </IconButton>
            }
        >
            <Collapse in={expanded} timeout="auto">
                <Box>
                    <Tabs
                        value={tab}
                        onChange={(_, v) => setTab(v)}
                        sx={{ minHeight: 32, mb: 1, '& .MuiTabs-indicator': { height: 3 } }}
                    >
                        {TABS.map((t) => (
                            <Tab
                                key={t.value}
                                value={t.value}
                                label={t.label}
                                sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }}
                            />
                        ))}
                    </Tabs>
                    <CodeBlock
                        code={code}
                        language={tab === 'curl' ? 'bash' : tab}
                        filename={active.filename}
                        onCopy={onCopy ? (c) => onCopy(c, active.filename) : undefined}
                        maxHeight={200}
                        wrap={false}
                    />
                </Box>
            </Collapse>
        </UnifiedCard>
    );
};

export default ImageGenQuickStartCard;
