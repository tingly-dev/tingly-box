import { ApiStyleBadge } from '@/components/ApiStyleBadge.tsx';
import {
    Box,
    Chip,
    Paper,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tooltip,
    Typography,
} from '@mui/material';
import type { Provider } from '../types/provider';

// Fixed chip width so rows form an aligned grid instead of ragged
// flex-wrap lines whose chip widths vary with label length.
const MODEL_CHIP_WIDTH = 132;

interface VirtualModelsTableProps {
    providers: Provider[];
    onToggle?: (providerUuid: string) => void;
}

// VirtualModelsTable renders the builtin virtual-model providers as a
// read-only list with a single actionable control: the enabled toggle.
// Builtin providers are seeded by the backend on every startup so deletion
// would just race back; mutation of name/base/etc. is rejected by the
// CRUD layer. Only Enabled is user-controllable.
const VirtualModelsTable = ({ providers, onToggle }: VirtualModelsTableProps) => {
    return (
        <TableContainer component={Paper} variant="outlined">
            <Table size="small">
                <TableHead>
                    <TableRow>
                        <TableCell sx={{ width: '18%' }}>Name</TableCell>
                        <TableCell sx={{ width: '10%' }}>API Style</TableCell>
                        <TableCell>Models</TableCell>
                        <TableCell sx={{ width: '12%' }} align="center">Enabled</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((p) => {
                        const models = p.vmodel_detail?.models ?? [];
                        return (
                            <TableRow key={p.uuid ?? p.name} hover>
                                <TableCell>
                                    <Stack direction="row" spacing={1} alignItems="center">
                                        <Typography variant="body2" fontWeight={500}>
                                            {p.name}
                                        </Typography>
                                        <Chip
                                            label="Builtin"
                                            size="small"
                                            variant="outlined"
                                            sx={{ height: 18, color: 'text.secondary' }}
                                        />
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <ApiStyleBadge apiStyle={p.api_style as any} compact />
                                </TableCell>
                                <TableCell>
                                    {models.length === 0 ? (
                                        <Typography variant="caption" color="text.secondary">
                                            none registered
                                        </Typography>
                                    ) : (
                                        <Box
                                            sx={{
                                                display: 'grid',
                                                gridTemplateColumns: `repeat(auto-fill, ${MODEL_CHIP_WIDTH}px)`,
                                                gap: 0.5,
                                            }}
                                        >
                                            {models.map((m: string) => (
                                                <Tooltip key={m} title={m}>
                                                    <Chip
                                                        label={m}
                                                        size="small"
                                                        variant="outlined"
                                                        sx={{
                                                            height: 20,
                                                            width: MODEL_CHIP_WIDTH,
                                                            '& .MuiChip-label': {
                                                                overflow: 'hidden',
                                                                textOverflow: 'ellipsis',
                                                            },
                                                        }}
                                                    />
                                                </Tooltip>
                                            ))}
                                        </Box>
                                    )}
                                </TableCell>
                                <TableCell align="center">
                                    <Tooltip title={p.enabled ? 'Disable virtual models' : 'Enable virtual models'}>
                                        <Switch
                                            size="small"
                                            checked={!!p.enabled}
                                            disabled={!p.uuid}
                                            onChange={() => {
                                                if (p.uuid) onToggle?.(p.uuid);
                                            }}
                                        />
                                    </Tooltip>
                                </TableCell>
                            </TableRow>
                        );
                    })}
                </TableBody>
            </Table>
        </TableContainer>
    );
};

export default VirtualModelsTable;
