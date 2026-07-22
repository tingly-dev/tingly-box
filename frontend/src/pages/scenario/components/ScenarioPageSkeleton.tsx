import { Box, Skeleton } from '@mui/material';

const ROUNDED_SX = { borderRadius: 2 } as const;

/**
 * Shape-matching placeholder for the "Use <Agent>" scenario pages (config
 * card + model rules list), shown while `useScenarioPageInternal` is
 * loading. Mirrors UnifiedCard's chrome (1px divider border, radius 2,
 * 24px padding) so the swap to real content doesn't cause a layout jump —
 * same pattern as DashboardPage's DashboardSkeleton. Deliberately full
 * width, no max-width/centering: the real CardGrid content it stands in
 * for isn't centered either, so a narrower/centered skeleton would jump
 * sideways the instant real content replaces it.
 */
export const ScenarioPageSkeleton: React.FC<{ ruleRows?: number }> = ({ ruleRows = 2 }) => (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 3 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                <Skeleton variant="text" width={160} height={32} />
                <Skeleton variant="rounded" width={100} height={32} sx={ROUNDED_SX} />
            </Box>
            <Skeleton variant="rounded" height={44} sx={{ ...ROUNDED_SX, mb: 1 }} />
            <Skeleton variant="rounded" height={44} sx={ROUNDED_SX} />
        </Box>

        <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 2, p: 3 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                <Skeleton variant="text" width={140} height={32} />
                <Box sx={{ display: 'flex', gap: 1 }}>
                    <Skeleton variant="rounded" width={90} height={32} sx={ROUNDED_SX} />
                    <Skeleton variant="rounded" width={90} height={32} sx={ROUNDED_SX} />
                </Box>
            </Box>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                {Array.from({ length: ruleRows }).map((_, i) => (
                    <Skeleton key={i} variant="rounded" height={120} sx={ROUNDED_SX} />
                ))}
            </Box>
        </Box>
    </Box>
);

export default ScenarioPageSkeleton;
