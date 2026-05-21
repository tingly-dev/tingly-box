import { useState } from 'react';
import { Box, Collapse, IconButton, Tab, Tabs } from '@mui/material';
import { ExpandLess, ExpandMore } from '@mui/icons-material';
import UnifiedCard from '@/components/UnifiedCard';
import CodeBlock from '@/components/CodeBlock';

interface TranslateQuickStartCardProps {
    baseUrl: string;
    model?: string;
    onCopy?: (text: string, label: string) => void;
}

type Lang = 'python' | 'curl';

const TABS: { value: Lang; label: string; filename: string }[] = [
    { value: 'python', label: 'Python', filename: 'translate.py' },
    { value: 'curl', label: 'curl', filename: 'translate.sh' },
];

const buildSnippet = (lang: Lang, baseUrl: string, model: string): string => {
    const endpoint = `${baseUrl}/tingly/translate/v1`;
    switch (lang) {
        case 'python':
            return `import requests

resp = requests.post(
    "${endpoint}/translations",
    headers={
        "Authorization": "Bearer <TINGLY_MODEL_TOKEN>",  # GET /api/v1/token
        "Content-Type": "application/json",
    },
    json={
        "model": "${model}",
        "input": "Hello, how are you?",
        "source_lang": "en",
        "target_lang": "zh",
    },
)
resp.raise_for_status()
data = resp.json()
print(data["translation"])
`;
        case 'curl':
            return `curl -X POST "${endpoint}/translations" \\
  -H "Authorization: Bearer <TINGLY_MODEL_TOKEN>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "input": "Hello, how are you?",
    "source_lang": "en",
    "target_lang": "zh"
  }'
`;
    }
};

const TranslateQuickStartCard: React.FC<TranslateQuickStartCardProps> = ({ baseUrl, model = 'Helsinki-NLP/opus-mt-en-zh', onCopy }) => {
    const [lang, setLang] = useState<Lang>('python');
    const [expanded, setExpanded] = useState(true);

    const currentTab = TABS.find(t => t.value === lang)!;
    const snippet = buildSnippet(lang, baseUrl, model);

    return (
        <UnifiedCard
            title={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <span>Quick Start</span>
                </Box>
            }
            size="full"
            rightAction={
                <IconButton size="small" onClick={() => setExpanded(v => !v)}>
                    {expanded ? <ExpandLess /> : <ExpandMore />}
                </IconButton>
            }
        >
            <Collapse in={expanded}>
                <Tabs value={lang} onChange={(_, v) => setLang(v)} sx={{ mb: 1 }}>
                    {TABS.map(t => (
                        <Tab key={t.value} value={t.value} label={t.label} />
                    ))}
                </Tabs>
                <CodeBlock
                    code={snippet}
                    language={lang === 'curl' ? 'bash' : lang}
                    filename={currentTab.filename}
                    onCopy={onCopy ? () => onCopy(snippet, currentTab.filename) : undefined}
                />
            </Collapse>
        </UnifiedCard>
    );
};

export default TranslateQuickStartCard;
