import {OpenAI, Anthropic} from '../BrandIcons';
import {Box, Checkbox, FormControlLabel, Link, Stack, TextField, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import ProviderIcon from '@/components/ProviderIcon';
import {fontMono} from '@/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

export interface ProtocolSlotData {url: string; enabled: boolean;}
export type ProtocolKind = 'openai' | 'anthropic';
interface ProtocolSlotProps {
    kind: ProtocolKind | 'google';
    selectionMode?: 'multiple' | 'fixed';
    slot: ProtocolSlotData;
    onUrlChange: (url: string) => void;
    onUrlBlur: () => void;
    onToggle: () => void;
    helperText?: string;
    urlError?: boolean;
    v1Hint?: {show: boolean; onApply: () => void};
}
const ProtocolSlot = ({kind, slot, onUrlChange, onUrlBlur, onToggle, helperText, urlError, v1Hint, selectionMode = 'multiple'}: ProtocolSlotProps) => {
    const {t} = useTranslation();
    const label = kind === 'google' ? t('providerDialog.editUx.googleCompatible') : kind === 'openai' ? t('providerDialog.protocol.openAILabel') : t('providerDialog.protocol.anthropicLabel');
    const helper = helperText || (kind === 'google' ? t('providerDialog.editUx.googleHint') : kind === 'openai' ? t('providerDialog.protocol.helperOpenAI') : t('providerDialog.protocol.helperAnthropic'));
    const header = <Stack direction="row" spacing={1} sx={{alignItems: 'flex-start'}}>
        <Box sx={{mt: 0.25, flexShrink: 0}}>{kind === 'google' ? <ProviderIcon identifier="google" size={18}/> : kind === 'openai' ? <OpenAI size={18}/> : <Anthropic size={18}/>}</Box>
        <Box><Typography variant="body2" sx={{fontWeight: 600}}>{label}</Typography>
            <Typography variant="caption" color="text.secondary" sx={{display: 'block', mt: 0.25}}>{helper}</Typography></Box>
    </Stack>;
    return <Box sx={{p: 1.5, border: 1, borderColor: urlError ? 'error.main' : 'divider', borderRadius: radius.card, bgcolor: slot.enabled ? 'action.hover' : 'transparent'}}>
        {selectionMode === 'fixed' ? header : <FormControlLabel labelPlacement="start" sx={{m: 0, width: '100%', gap: 1, justifyContent: 'space-between', alignItems: 'flex-start'}}
            control={<Checkbox size="small" checked={slot.enabled} onChange={onToggle} sx={{p: 0.25}}/>}
            label={header}/>}
        {slot.enabled && <Stack spacing={1} sx={{mt: 1.5}}>
            <TextField fullWidth size="small" type="url" required
                label={kind === 'google' ? t('providerDialog.provider.placeholder') : kind === 'openai' ? t('providerDialog.editUx.openAIUrl') : t('providerDialog.editUx.anthropicUrl')}
                value={slot.url} onChange={e => onUrlChange(e.target.value)} onBlur={onUrlBlur}
                placeholder="https://api.example.com" error={urlError}
                helperText={urlError ? t('providerDialog.editUx.urlError') : undefined}
                slotProps={{htmlInput: {spellCheck: false, autoComplete: 'off'}, input: {sx: {fontFamily: fontMono, bgcolor: 'background.paper'}}}}/>
            {v1Hint?.show && <Box sx={{display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'baseline'}}>
                <Typography variant="caption" color="text.secondary">{t('providerDialog.v1Hint.message')}</Typography>
                <Link component="button" type="button" variant="caption" onClick={v1Hint.onApply}>{t('providerDialog.v1Hint.apply')}</Link>
            </Box>}
        </Stack>}
    </Box>;
};
export default ProtocolSlot;
