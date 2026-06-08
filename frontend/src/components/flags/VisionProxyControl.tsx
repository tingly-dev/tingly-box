import { Check as IconCheck, KeyboardArrowDown as IconChevronDown } from '@/components/icons';
import {
    Box,
    Button,
    Dialog,
    DialogContent,
    DialogTitle,
    ListItemText,
    Menu,
    MenuItem,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useState } from 'react';
import ModelSelectDialog from '../ModelSelectDialog';
import type { ProviderSelectTabOption } from '../ModelSelectDialog';
import type { Provider } from '@/types/provider';

export interface VisionService {
    provider: string;
    model: string;
}

interface VisionProxyControlProps {
    value: VisionService | null;
    providers: Provider[];
    disabled?: boolean;
    onChange: (service: VisionService | null) => void;
}

const VisionProxyControl: React.FC<VisionProxyControlProps> = ({ value, providers, disabled, onChange }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const [pickerOpen, setPickerOpen] = useState(false);

    const providerName = (uuid: string) => providers.find(p => p.uuid === uuid)?.name || uuid;
    const isEnabled = !!value;
    const label = isEnabled ? value!.model : 'Off';
    const tooltip = isEnabled
        ? `Vision Proxy: images described by ${providerName(value!.provider)} / ${value!.model}`
        : 'Vision Proxy: describe images via a vision-capable model so text-only downstreams can read them';

    return (
        <>
            <Tooltip title={tooltip} placement="right" arrow>
                <Button
                    size="small"
                    variant="outlined"
                    onClick={(e) => !disabled && setAnchor(e.currentTarget)}
                    disabled={disabled}
                    endIcon={<IconChevronDown sx={{ fontSize: 18 }} />}
                    sx={{
                        minWidth: 100,
                        maxWidth: 260,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        '& .MuiButton-endIcon': { flexShrink: 0 },
                        bgcolor: isEnabled ? 'primary.main' : 'transparent',
                        color: isEnabled ? 'primary.contrastText' : 'text.primary',
                        fontWeight: isEnabled ? 600 : 400,
                        border: isEnabled ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: disabled ? 0.6 : 1,
                        '&:hover': { bgcolor: isEnabled ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                        Vision Proxy: {label}
                    </Box>
                </Button>
            </Tooltip>

            <Menu
                anchorEl={anchor}
                open={Boolean(anchor)}
                onClose={() => setAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                <MenuItem
                    selected={!isEnabled}
                    onClick={() => { setAnchor(null); if (isEnabled) onChange(null); }}
                >
                    <ListItemText primary="Off" primaryTypographyProps={{ variant: 'body2' }} />
                    {!isEnabled && <IconCheck sx={{ fontSize: 16 }} />}
                </MenuItem>
                <MenuItem
                    selected={isEnabled}
                    onClick={() => { setAnchor(null); setPickerOpen(true); }}
                >
                    <ListItemText
                        primary={isEnabled ? `On — ${value!.model}` : 'On — pick a model…'}
                        secondary={isEnabled ? providerName(value!.provider) : 'Choose a vision-capable model'}
                        primaryTypographyProps={{ variant: 'body2' }}
                        secondaryTypographyProps={{ variant: 'caption' }}
                    />
                    {isEnabled && <IconCheck sx={{ fontSize: 16 }} />}
                </MenuItem>
            </Menu>

            <Dialog
                open={pickerOpen}
                onClose={() => setPickerOpen(false)}
                maxWidth="lg"
                fullWidth
                PaperProps={{ sx: { height: '80vh' } }}
            >
                <DialogTitle sx={{ textAlign: 'center' }}>
                    <Typography variant="h6">Pick Vision Proxy Model</Typography>
                </DialogTitle>
                <DialogContent>
                    <ModelSelectDialog
                        providers={providers}
                        selectedProvider={value?.provider}
                        selectedModel={value?.model}
                        onSelected={async (option: ProviderSelectTabOption) => {
                            onChange({ provider: option.provider.uuid, model: option.model });
                            setPickerOpen(false);
                        }}
                        onSelectionClear={async () => {
                            onChange(null);
                            setPickerOpen(false);
                        }}
                    />
                </DialogContent>
            </Dialog>
        </>
    );
};

export default VisionProxyControl;
