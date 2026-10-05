import {
    Box,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Button,
    Typography,
    Stack,
    Link,
    TextField,
    IconButton,
    CircularProgress,
    Tooltip,
} from '@mui/material';
import React, { useState } from 'react';
import { ContentCopy as ContentCopyIcon } from '@/components/icons';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import { scrollToModelsCard } from './AgentSetupCard';
import Context1MChangeBanner from './Context1MChangeBanner';
import { CopyUrlKeyButtons } from './config/CopyUrlKeyButtons';
import api from '@/services/api';
import { fontMono, fontSizes } from '@/theme/fonts';

interface ClaudeDesktopConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    rules?: any[];
    onRulesRefresh?: () => void;
    pendingContext1MChange?: boolean | null;
}

const LABEL_PREFIX = 'label:';
const labelOf = (rule: any): string =>
    rule.description?.startsWith(LABEL_PREFIX) ? rule.description.slice(LABEL_PREFIX.length) : '';

const buildInferenceModelsJson = (modelRules: any[]): string => {
    const entries = modelRules.map(r => {
        const label = labelOf(r);
        if (label) {
            return `    {\n      "name": "${r.request_model}",\n      "labelOverride": "${label}"\n    }`;
        }
        return `    {\n      "name": "${r.request_model}"\n    }`;
    });
    return `"inferenceModels": [\n${entries.join(',\n')}\n  ]`;
};

const EMPTY_RULES: any[] = [];

const ClaudeDesktopConfigModal: React.FC<ClaudeDesktopConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    copyToClipboard,
    rules = EMPTY_RULES,
    onRulesRefresh,
    pendingContext1MChange,
}) => {
    const { token } = useScenarioPageModal();
    // Label edits in progress, by rule uuid; saved on blur / Enter.
    const [labelDrafts, setLabelDrafts] = useState<Record<string, string>>({});
    const [savingUuid, setSavingUuid] = useState<string | null>(null);

    const modelRules = rules.filter(r => r.request_model && r.request_model !== '*');
    const inferenceModelsJson = buildInferenceModelsJson(modelRules);

    // The models are the rules in Model Rules; this dialog only adds the
    // Claude-Desktop-specific display label (labelOverride), which is stored
    // in the rule's description. Rule updates replace the whole record, so
    // the fetched rule is sent back with only the description changed.
    const saveLabel = async (rule: any) => {
        const draft = labelDrafts[rule.uuid];
        if (draft === undefined) return;
        const label = draft.trim();
        if (label !== labelOf(rule)) {
            setSavingUuid(rule.uuid);
            try {
                await api.updateRule(rule.uuid, { ...rule, description: label ? `${LABEL_PREFIX}${label}` : '' });
                onRulesRefresh?.();
            } finally {
                setSavingUuid(null);
            }
        }
        setLabelDrafts(({ [rule.uuid]: _, ...rest }) => rest);
    };

    const editModelsInRules = () => {
        onClose();
        scrollToModelsCard();
    };

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            slotProps={{
                paper: { sx: { borderRadius: 3 } }
            }}
        >
            <DialogTitle sx={{ pb: 1 }}>
                <Typography variant="h6" sx={{
                    fontWeight: 600
                }}>
                    Configure Claude Desktop
                </Typography>
            </DialogTitle>
            <DialogContent sx={{ pt: 1 }}>
                {pendingContext1MChange != null && (
                    <Context1MChangeBanner enabled={pendingContext1MChange} clientName="Claude Desktop" requiresApply={false} />
                )}
                <Stack spacing={2}>
                    {/* Step 1 */}
                    <Box sx={{ p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 1: Enable Developer Mode
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
                            Download Claude Desktop from{' '}
                            <Link href="https://claude.com/download" target="_blank" underline="hover">
                                claude.com/download
                            </Link>
                        </Typography>
                        <Typography variant="subtitle2" sx={{ mb: 1 }}>
                            Launch the app, then enable developer mode:
                        </Typography>
                        <Box sx={{ bgcolor: 'background.default', p: 1.5, borderRadius: 1 }}>
                            <Typography variant="body2" sx={{ fontFamily: fontMono, fontSize: fontSizes.md }}>
                                Help → Troubleshooting → Enable Developer Mode
                            </Typography>
                        </Box>
                    </Box>

                    {/* Step 2 */}
                    <Box sx={{ p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Typography variant="subtitle2" sx={{ mb: 1.5, fontWeight: 600 }}>
                            Step 2: Configure Third-Party Inference
                        </Typography>
                        <Box sx={{ bgcolor: 'background.default', p: 1.5, borderRadius: 1, mb: 1.5 }}>
                            <Typography variant="body2" sx={{ fontFamily: fontMono, fontSize: fontSizes.md }}>
                                Developer → Configure third-party inference
                            </Typography>
                        </Box>
                        <Box sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '4px 12px', alignItems: 'baseline' }}>
                            <Typography variant="subtitle2"><strong>Connection:</strong></Typography>
                            <Typography variant="subtitle2">Gateway</Typography>
                            <Typography variant="subtitle2"><strong>Base URL:</strong></Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: fontMono, wordBreak: 'break-all' }}>
                                {baseUrl}/tingly/claude_desktop
                            </Typography>
                            <Typography variant="subtitle2"><strong>API key:</strong></Typography>
                            <Typography variant="subtitle2" sx={{ fontFamily: fontMono }}>
                                {token.slice(0, 16)}…
                            </Typography>
                        </Box>
                    </Box>

                    {/* Copy buttons */}
                    <CopyUrlKeyButtons
                        url={`${baseUrl}/tingly/claude_desktop`}
                        token={token}
                        copyToClipboard={copyToClipboard}
                    />

                    {/* Step 3 */}
                    <Box sx={{ p: 2, borderRadius: 1, border: 1, borderColor: 'divider' }}>
                        <Stack
                            direction="row"
                            sx={{
                                justifyContent: "space-between",
                                alignItems: "center",
                                mb: 1
                            }}>
                            <Typography variant="subtitle2" sx={{
                                fontWeight: 600
                            }}>
                                Step 3: Configure Models
                            </Typography>
                            {modelRules.length > 0 && (
                                <Tooltip title="Copy inferenceModels JSON">
                                    <IconButton
                                        size="small"
                                        onClick={() => copyToClipboard(inferenceModelsJson, 'inferenceModels')}
                                    >
                                        <ContentCopyIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            )}
                        </Stack>

                        <Typography
                            variant="body2"
                            sx={{
                                color: "text.secondary",
                                mb: 0.5
                            }}>
                            <strong>Optional</strong> — Claude Desktop will auto-discover models from{' '}
                            <code>/v1/models</code> if left empty.
                        </Typography>
                        <Typography
                            variant="body2"
                            sx={{
                                color: "text.secondary",
                                mb: 1.5
                            }}>
                            To pin a specific list, paste the JSON below into the <em>inferenceModels</em> field.
                            Custom model names may not be recognized by Claude Desktop.
                        </Typography>

                        {/* JSON preview */}
                        <Box
                            sx={{
                                bgcolor: 'background.default',
                                borderRadius: 1,
                                p: 1.5,
                                mb: 2,
                                fontFamily: fontMono,
                                fontSize: fontSizes.md,
                                lineHeight: 1.6,
                                whiteSpace: 'pre',
                                overflowX: 'auto',
                                color: modelRules.length === 0 ? 'text.disabled' : 'text.primary',
                            }}
                        >
                            {modelRules.length === 0
                                ? '"inferenceModels": []'
                                : inferenceModelsJson}
                        </Box>

                        {/* One row per model rule: its name, and the label Claude
                            Desktop shows for it. Models themselves are added and
                            removed in Model Rules, the one place rules are edited. */}
                        <Stack spacing={0.5} sx={{ mb: 1.5 }}>
                            {modelRules.map(rule => (
                                <Stack
                                    key={rule.uuid}
                                    direction="row"
                                    spacing={1}
                                    sx={{ alignItems: 'center', bgcolor: 'background.default', borderRadius: 1, px: 1.5, py: 0.5 }}>
                                    <Typography sx={{ fontFamily: fontMono, fontSize: fontSizes.md, flex: 2, minWidth: 0 }}>
                                        {rule.request_model}
                                    </Typography>
                                    <TextField
                                        size="small"
                                        variant="standard"
                                        placeholder="label (optional)"
                                        value={labelDrafts[rule.uuid] ?? labelOf(rule)}
                                        onChange={e => setLabelDrafts(d => ({ ...d, [rule.uuid]: e.target.value }))}
                                        onBlur={() => void saveLabel(rule)}
                                        onKeyDown={e => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
                                        disabled={savingUuid === rule.uuid}
                                        sx={{ flex: 1 }}
                                        slotProps={{ htmlInput: { style: { fontSize: fontSizes.md }, 'aria-label': `Label for ${rule.request_model}` } }}
                                    />
                                    {savingUuid === rule.uuid && <CircularProgress size={14} />}
                                </Stack>
                            ))}
                        </Stack>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                            <Typography variant="body2" sx={{ color: 'text.secondary', flex: 1 }}>
                                Models come from the rules on this page — add or remove them in Model Rules.
                            </Typography>
                            <Button size="small" onClick={editModelsInRules} sx={{ flexShrink: 0 }}>
                                Edit in Model Rules
                            </Button>
                        </Stack>
                    </Box>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
                <Button onClick={onClose} variant="contained">Done</Button>
            </DialogActions>
        </Dialog>
    );
};

export default ClaudeDesktopConfigModal;
