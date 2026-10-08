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
    Typography,
} from '@mui/material';
import { Close } from '@/components/icons';
import CodeBlock from '@/components/CodeBlock';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';

interface DecisionsGuideModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    /** Request model of the first rule, when one exists. */
    model?: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

type Lang = 'python' | 'typescript' | 'curl';

const TABS: { value: Lang; label: string }[] = [
    { value: 'python', label: 'Python' },
    { value: 'typescript', label: 'TypeScript' },
    { value: 'curl', label: 'curl' },
];

const FILENAMES: Record<Lang, string> = {
    python: 'decisions.py',
    typescript: 'decisions.ts',
    curl: 'decisions.sh',
};

// The request body is opaque to the gateway (the upstream schema differs per
// vendor); this is the choice-question shape used by Jev / TypeSafe.
// See .design/decision-protocol.md.
const STATE = 'My invoice shows a charge I do not recognise.';
const INSTRUCTIONS = 'Which team should handle this ticket?';

const buildSnippet = (lang: Lang, endpoint: string, model: string, token: string): string => {
    switch (lang) {
        case 'python':
            return `# pip install openai
from openai import OpenAI

client = OpenAI(
    base_url="${endpoint}",
    api_key="${token}",
)

resp = client.post(
    "/decisions",
    body={
        "model": "${model}",
        "state": "${STATE}",
        "questions": {
            "team": {
                "type": "choice",
                "instructions": "${INSTRUCTIONS}",
                "criteria": {
                    "billing": "Charges, invoices, refunds",
                    "technical": "Bugs, errors, outages",
                },
            }
        },
    },
    cast_to=object,
)
print(resp["answers"]["team"]["choice"])
`;
        case 'typescript':
            return `// npm i openai
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${endpoint}",
  apiKey: "${token}",
});

const resp: any = await client.post("/decisions", {
  body: {
    model: "${model}",
    state: "${STATE}",
    questions: {
      team: {
        type: "choice",
        instructions: "${INSTRUCTIONS}",
        criteria: {
          billing: "Charges, invoices, refunds",
          technical: "Bugs, errors, outages",
        },
      },
    },
  },
});
console.log(resp.answers.team.choice);
`;
        case 'curl':
            return `curl ${endpoint}/decisions \\
  -H "Authorization: Bearer ${token}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "state": "${STATE}",
    "questions": {
      "team": {
        "type": "choice",
        "instructions": "${INSTRUCTIONS}",
        "criteria": {
          "billing": "Charges, invoices, refunds",
          "technical": "Bugs, errors, outages"
        }
      }
    }
  }'
`;
    }
};

const DecisionsGuideModal: React.FC<DecisionsGuideModalProps> = ({
    open,
    onClose,
    baseUrl,
    model,
    copyToClipboard,
}) => {
    const { t } = useTranslation();
    const { token } = useScenarioPageModal();
    const [tab, setTab] = useState<Lang>('python');

    const endpoint = `${baseUrl}/tingly/decisions/v1`;
    const code = buildSnippet(tab, endpoint, model || 'my-decision-model', token || '<TINGLY_MODEL_TOKEN>');
    const filename = FILENAMES[tab];

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="md"
            fullWidth
            slotProps={{ paper: { sx: { borderRadius: 3 } } }}
        >
            <DialogTitle sx={{ pb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography component="span" variant="h6" sx={{ fontWeight: 600 }}>
                    {t('decisionsGuide.title')}
                </Typography>
                <IconButton onClick={onClose} size="small" aria-label={t('decisionsGuide.closeAriaLabel')}>
                    <Close fontSize="small" />
                </IconButton>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
                    {t('decisionsGuide.description')}
                </Typography>
                <Tabs
                    value={tab}
                    onChange={(_, value: Lang) => setTab(value)}
                    sx={{ minHeight: 36, mb: 1, '& .MuiTabs-indicator': { height: 3 } }}
                >
                    {TABS.map((item) => (
                        <Tab key={item.value} value={item.value} label={item.label} sx={{ minHeight: 36, py: 0.5 }} />
                    ))}
                </Tabs>
                <Box>
                    <CodeBlock
                        code={code}
                        language={tab === 'curl' ? 'bash' : tab}
                        filename={filename}
                        onCopy={(content) => { void copyToClipboard(content, filename); }}
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

export default DecisionsGuideModal;
