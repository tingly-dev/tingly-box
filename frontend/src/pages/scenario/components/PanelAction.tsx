import { Box, Button, Tooltip } from '@mui/material';
import { panelActionLabelSx, panelActionSx } from './ImageGenPlayground.chrome';

// A results-panel header action. Three of them do not fit a phone at full
// width, so the label drops away below `sm` and the icon carries the button —
// the action itself never disappears, and the aria-label keeps saying what it
// is.
const PanelAction: React.FC<{
    label: string;
    icon: React.ReactNode;
    onClick: () => void;
    testId?: string;
}> = ({ label, icon, onClick, testId }) => (
    <Tooltip title={label}>
        <Button
            size="small"
            color="inherit"
            startIcon={icon}
            onClick={onClick}
            aria-label={label}
            data-testid={testId}
            sx={panelActionSx}
        >
            <Box component="span" sx={panelActionLabelSx}>{label}</Box>
        </Button>
    </Tooltip>
);

export default PanelAction;
