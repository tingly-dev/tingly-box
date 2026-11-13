import { Box, Button, Popover, Typography } from '@mui/material';
import { Extension as ExtensionIcon } from '@/components/icons';
import React from 'react';
import { useTranslation } from 'react-i18next';
import type { FlagSpec, RuleFlags } from '@/components/RoutingGraphTypes';
import { fontSizes } from '@/theme/fonts';
import { RulePluginsList } from './RulePluginsCard';

export interface RulePluginsPopoverProps {
    anchorEl: HTMLElement | null;
    onClose: () => void;
    flags?: RuleFlags;
    /** The plugins that are on for this rule. */
    enabled: FlagSpec[];
    /** A row opens the catalog on that plugin; no key opens it from the top. */
    onOpenCatalog: (flagKey?: string) => void;
    onRemoveFlag?: (key: string) => void;
}

/**
 * A rule's plugins as a popover over the rule header (opened from its
 * "Plugins N" tag), instead of a ~240px column pinned to the right of the
 * routing graph. Same rows as the old card; the footer button opens the
 * catalog for adding more.
 */
export const RulePluginsPopover: React.FC<RulePluginsPopoverProps> = ({
    anchorEl,
    onClose,
    flags,
    enabled,
    onOpenCatalog,
    onRemoveFlag,
}) => {
    const { t } = useTranslation();
    return (
        <Popover
            // Removing the last plugin removes the tag it hangs from: close with it.
            open={Boolean(anchorEl) && enabled.length > 0}
            anchorEl={anchorEl}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            slotProps={{ paper: { sx: { width: 300, maxWidth: 'calc(100vw - 32px)', p: 1.25 } } }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, mb: 0.75 }}>
                <ExtensionIcon sx={{ fontSize: 14, color: 'text.secondary' }} />
                <Typography variant="caption" sx={{ fontWeight: 600, fontSize: fontSizes.micro, color: 'text.secondary' }}>
                    {t('ruleHeader.plugins')} ({enabled.length})
                </Typography>
            </Box>
            <Box sx={{ maxHeight: 240, overflowY: 'auto' }}>
                <RulePluginsList
                    flags={flags}
                    enabled={enabled}
                    onOpenCatalog={(key) => { onClose(); onOpenCatalog(key); }}
                    onRemoveFlag={onRemoveFlag}
                />
            </Box>
            <Button
                size="small"
                fullWidth
                onClick={() => { onClose(); onOpenCatalog(); }}
                sx={{ mt: 1, textTransform: 'none' }}
            >
                {t('ruleHeader.managePlugins')}
            </Button>
        </Popover>
    );
};

export default RulePluginsPopover;
