import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    List,
    ListItemButton,
    ListItemText,
    Paper,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { AutoFixHigh as AutoFixHighIcon } from '@/components/icons';
import { Link as LinkIcon } from '@/components/icons';
import { VpnKey as VpnKeyIcon } from '@/components/icons';
import {extractOnboardingCandidates, type OnboardingTokenCandidate} from '@/services/onboardingExtract';
import type {EnhancedProviderFormData} from '@/components/ProviderFormDialog';

interface PasteAndDetectProps {
    onPick: (prefill: EnhancedProviderFormData) => void;
    onManualFill: () => void;
}

const PLACEHOLDER = `Paste anything: a .env snippet, a curl command, a JSON config…

# .env
OPENAI_API_KEY=sk-proj-...
OPENAI_BASE_URL=https://api.openai.com/v1

curl https://api.anthropic.com/v1/messages \\
  -H "x-api-key: sk-ant-..."
`;

const PasteAndDetect: React.FC<PasteAndDetectProps> = ({onPick, onManualFill}) => {
    const {t} = useTranslation();
    const [input, setInput] = useState('');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [urls, setUrls] = useState<string[] | null>(null);
    const [tokens, setTokens] = useState<OnboardingTokenCandidate[] | null>(null);
    const [selectedURL, setSelectedURL] = useState<string | null>(null);
    const [selectedToken, setSelectedToken] = useState<string | null>(null);

    const handleDetect = async () => {
        setLoading(true);
        setError(null);
        try {
            const res = await extractOnboardingCandidates(input);
            if (!res.success) {
                setError(res.error || 'Extraction failed');
                setUrls([]);
                setTokens([]);
                setSelectedURL(null);
                setSelectedToken(null);
                return;
            }
            setUrls(res.urls);
            setTokens(res.tokens);
            // Sensible default: pre-select if there's exactly one of each.
            setSelectedURL(res.urls.length === 1 ? res.urls[0] : null);
            setSelectedToken(res.tokens.length === 1 ? res.tokens[0].value : null);
        } finally {
            setLoading(false);
        }
    };

    const handleUseSelected = () => {
        // Vendor-agnostic: just hand the chosen URL+token to the dialog.
        // ProviderFormDialog already matches a known provider against
        // apiBase, fills the rest of the form, and lets the user override
        // anything before saving.
        onPick({
            name: '',
            apiBase: selectedURL || '',
            apiStyle: undefined,
            token: selectedToken || '',
            enabled: true,
        });
    };

    const canUse = !!selectedURL || !!selectedToken;
    const hasResults = urls !== null;

    return (
        <Box>
            <TextField
                fullWidth
                multiline
                minRows={6}
                maxRows={14}
                value={input}
                onChange={e => setInput(e.target.value)}
                placeholder={PLACEHOLDER}
                spellCheck={false}
                inputProps={{style: {fontFamily: 'monospace', fontSize: 13}}}
            />

            <Stack direction="row" spacing={1.5} sx={{mt: 1.5}}>
                <Button
                    variant="contained"
                    startIcon={loading ? <CircularProgress size={16} color="inherit"/> : <AutoFixHighIcon/>}
                    onClick={handleDetect}
                    disabled={loading || !input.trim()}
                >
                    {t('onboarding.paste.detectButton', {defaultValue: 'Detect'})}
                </Button>
                <Button variant="text" onClick={onManualFill}>
                    {t('onboarding.paste.manualFill', {defaultValue: 'Fill in manually'})}
                </Button>
            </Stack>

            {error && (
                <Alert severity="error" sx={{mt: 2}}>
                    {error}
                </Alert>
            )}

            {hasResults && (
                <Box sx={{mt: 2}}>
                    {urls!.length === 0 && tokens!.length === 0 ? (
                        <Alert severity="info">
                            {t('onboarding.paste.noMatch', {
                                defaultValue: 'No URL or API key detected. You can fill in the form manually.',
                            })}
                        </Alert>
                    ) : (
                        <>
                            <Typography variant="caption" color="text.secondary" sx={{display: 'block', mb: 1.5}}>
                                {t('onboarding.paste.pickHint', {
                                    defaultValue: 'Pick the URL and the token you want to use, then click "Use selected".',
                                })}
                            </Typography>

                            <Stack direction={{xs: 'column', md: 'row'}} spacing={2}>
                                <Paper variant="outlined" sx={{flex: 1, p: 1.5, minWidth: 0}}>
                                    <Stack direction="row" spacing={1} alignItems="center" sx={{mb: 1}}>
                                        <LinkIcon fontSize="small" color="action"/>
                                        <Typography variant="subtitle2" fontWeight={600}>
                                            {t('onboarding.paste.urlsTitle', {defaultValue: 'Detected URLs'})}
                                        </Typography>
                                        <Chip label={urls!.length} size="small" sx={{height: 18, fontSize: '0.65rem'}}/>
                                    </Stack>
                                    {urls!.length === 0 ? (
                                        <Typography variant="caption" color="text.secondary">
                                            {t('onboarding.paste.noURL', {defaultValue: 'No URLs detected.'})}
                                        </Typography>
                                    ) : (
                                        <List dense disablePadding>
                                            {urls!.map(u => (
                                                <ListItemButton
                                                    key={u}
                                                    selected={selectedURL === u}
                                                    onClick={() => setSelectedURL(prev => prev === u ? null : u)}
                                                    sx={{borderRadius: 1, mb: 0.5}}
                                                >
                                                    <ListItemText
                                                        primary={
                                                            <Typography
                                                                variant="body2"
                                                                sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}
                                                            >
                                                                {u}
                                                            </Typography>
                                                        }
                                                    />
                                                </ListItemButton>
                                            ))}
                                        </List>
                                    )}
                                </Paper>

                                <Paper variant="outlined" sx={{flex: 1, p: 1.5, minWidth: 0}}>
                                    <Stack direction="row" spacing={1} alignItems="center" sx={{mb: 1}}>
                                        <VpnKeyIcon fontSize="small" color="action"/>
                                        <Typography variant="subtitle2" fontWeight={600}>
                                            {t('onboarding.paste.tokensTitle', {defaultValue: 'Detected tokens'})}
                                        </Typography>
                                        <Chip label={tokens!.length} size="small" sx={{height: 18, fontSize: '0.65rem'}}/>
                                    </Stack>
                                    {tokens!.length === 0 ? (
                                        <Typography variant="caption" color="text.secondary">
                                            {t('onboarding.paste.noToken', {defaultValue: 'No tokens detected.'})}
                                        </Typography>
                                    ) : (
                                        <List dense disablePadding>
                                            {tokens!.map(tok => (
                                                <ListItemButton
                                                    key={tok.value}
                                                    selected={selectedToken === tok.value}
                                                    onClick={() => setSelectedToken(prev => prev === tok.value ? null : tok.value)}
                                                    sx={{borderRadius: 1, mb: 0.5}}
                                                >
                                                    <ListItemText
                                                        primary={
                                                            <Typography
                                                                variant="body2"
                                                                sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}
                                                            >
                                                                {tok.value}
                                                            </Typography>
                                                        }
                                                        secondary={
                                                            <Typography variant="caption" color="text.secondary">
                                                                {tok.source}
                                                            </Typography>
                                                        }
                                                    />
                                                </ListItemButton>
                                            ))}
                                        </List>
                                    )}
                                </Paper>
                            </Stack>

                            <Stack direction="row" spacing={1.5} sx={{mt: 2}}>
                                <Button
                                    variant="contained"
                                    onClick={handleUseSelected}
                                    disabled={!canUse}
                                >
                                    {t('onboarding.paste.useSelected', {defaultValue: 'Use selected'})}
                                </Button>
                                {(selectedURL || selectedToken) && (
                                    <Button
                                        variant="text"
                                        onClick={() => {
                                            setSelectedURL(null);
                                            setSelectedToken(null);
                                        }}
                                    >
                                        {t('common.clear', {defaultValue: 'Clear selection'})}
                                    </Button>
                                )}
                            </Stack>
                        </>
                    )}
                </Box>
            )}
        </Box>
    );
};

export default PasteAndDetect;
