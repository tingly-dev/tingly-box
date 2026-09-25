import {FolderOpen} from '@/components/icons';
import {Chip, Tooltip} from '@mui/material';
import {folderName} from './deskUtils';

// The folder a session works in: its name on the chip, the full path (the
// value the user would actually copy or compare) in the tooltip.
const FolderChip = ({path}: {path: string}) => (
    <Tooltip title={path}>
        <Chip
            size="small"
            variant="outlined"
            icon={<FolderOpen sx={{fontSize: 14}}/>}
            label={folderName(path)}
            sx={{color: 'text.secondary', fontFamily: 'monospace', '& .MuiChip-label': {px: 0.75}}}
        />
    </Tooltip>
);

export default FolderChip;
