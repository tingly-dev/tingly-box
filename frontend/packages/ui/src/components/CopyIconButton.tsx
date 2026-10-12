import { IconButton, Tooltip, type IconButtonProps, type TooltipProps } from '@mui/material';
import type { SxProps, Theme } from '@mui/material/styles';
import { Check, ContentCopy } from '../icons';
import { useCopyFeedback } from '../hooks/useCopyFeedback';

export interface CopyIconButtonProps {
    /** Text copied to the clipboard on click. */
    value: string;
    label?: string;
    copiedLabel?: string;
    /** sx `color` value for each state — a theme token, a CSS color, or 'inherit'. */
    color?: string;
    copiedColor?: string;
    size?: IconButtonProps['size'];
    edge?: IconButtonProps['edge'];
    /** Icon fontSize override (px, rem, ...); omit to use the MUI 'small' preset. */
    iconSize?: string | number;
    tooltipPlacement?: TooltipProps['placement'];
    tooltipArrow?: boolean;
    'aria-label'?: string;
    /** Merged into the IconButton's sx — for absolute positioning, hover, etc. */
    sx?: SxProps<Theme>;
    /** Runs once the copy actually succeeds — e.g. to also fire a toast. */
    onCopied?: () => void;
}

/**
 * The Tooltip + IconButton + ContentCopy⇄Check swap chrome that showed up
 * around `useCopyFeedback` in a dozen places — byte-identical in three of
 * them. Owns its own copy state, so it's for a standalone copy button only:
 * when the "copied" flag must be shared with a sibling element elsewhere on
 * the page (e.g. AccessControl's token row + its own reset-success banner),
 * wire `useCopyFeedback` directly instead.
 */
export const CopyIconButton = ({
    value,
    label = 'Copy',
    copiedLabel = 'Copied!',
    color = 'text.secondary',
    copiedColor = 'success.main',
    size = 'small',
    edge,
    iconSize,
    tooltipPlacement,
    tooltipArrow = false,
    'aria-label': ariaLabel,
    sx,
    onCopied,
}: CopyIconButtonProps) => {
    const { copied, copy } = useCopyFeedback();
    const iconSx = iconSize !== undefined ? { fontSize: iconSize } : undefined;

    return (
        <Tooltip title={copied ? copiedLabel : label} placement={tooltipPlacement} arrow={tooltipArrow}>
            <IconButton
                size={size}
                edge={edge}
                aria-label={ariaLabel ?? label}
                onClick={() => copy(value, onCopied)}
                sx={{ color: copied ? copiedColor : color, ...sx }}
            >
                {copied
                    ? <Check fontSize="small" sx={iconSx} />
                    : <ContentCopy fontSize="small" sx={iconSx} />}
            </IconButton>
        </Tooltip>
    );
};

export default CopyIconButton;
