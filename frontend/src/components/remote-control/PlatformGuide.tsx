import { OpenInNew } from '@mui/icons-material';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Box,
    Chip,
    Link,
    Stack,
    Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';

interface PlatformGuideProps {
    expanded: string | false;
    onChange: (panel: string) => (event: React.SyntheticEvent, isExpanded: boolean) => void;
}

interface PlatformConfig {
    id: string;
    name: string;
    icon: React.ReactNode;
    bgColor: string;
    status: 'available' | 'coming-soon' | 'beta';
    requiredFields: string[];
    steps: React.ReactNode;
}

const platformConfigs: PlatformConfig[] = [
    {
        id: 'telegram',
        name: 'Telegram',
        icon: <Typography sx={{ color: 'white', fontWeight: 'bold', fontSize: '0.75rem' }}>TG</Typography>,
        bgColor: '#0088cc',
        status: 'available',
        requiredFields: ['Bot Token'],
        steps: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Open Telegram, search{' '}
                        <Link href="https://t.me/BotFather" target="_blank">
                            @BotFather <OpenInNew sx={{ fontSize: 10 }} />
                        </Link>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Get token
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Send <code>/newbot</code>, follow the prompts, and copy the token
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        3. Configure
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Paste the token into the bot configuration form above
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        💡 Tip: Find your Chat ID
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Forward any message to <Link href="https://t.me/userinfobot" target="_blank">@userinfobot</Link> to get your Chat ID.
                        Use this in the "Chat ID Lock" field to restrict bot access to yourself.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    {
        id: 'wechat',
        name: 'WeChat',
        icon: <Typography sx={{ color: 'white', fontWeight: 'bold', fontSize: '0.75rem' }}>微信</Typography>,
        bgColor: '#07c160',
        status: 'coming-soon',
        requiredFields: ['App ID', 'App Secret'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                WeChat bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'discord',
        name: 'Discord',
        icon: <Typography sx={{ color: 'white', fontWeight: 'bold', fontSize: '0.75rem' }}>DC</Typography>,
        bgColor: '#5865F2',
        status: 'coming-soon',
        requiredFields: ['Bot Token', 'Message Content Intent'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                Discord bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'feishu',
        name: 'Feishu (飞书)',
        icon: <Typography sx={{ color: 'white', fontWeight: 'bold', fontSize: '0.75rem' }}>飞书</Typography>,
        bgColor: '#3370FF',
        status: 'coming-soon',
        requiredFields: ['App ID', 'App Secret'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                Feishu bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'dingtalk',
        name: 'DingTalk (钉钉)',
        icon: <Typography sx={{ color: 'white', fontWeight: 'bold', fontSize: '0.75rem' }}>钉钉</Typography>,
        bgColor: '#0089FF',
        status: 'coming-soon',
        requiredFields: ['App Key', 'App Secret'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                DingTalk bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'slack',
        name: 'Slack',
        icon: <Typography sx={{ color: 'white', fontWeight: 'bold', fontSize: '0.75rem' }}>SL</Typography>,
        bgColor: '#4A154B',
        status: 'coming-soon',
        requiredFields: ['Bot Token', 'App-Level Token'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                Slack bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
];

const PlatformGuide: React.FC<PlatformGuideProps> = ({ expanded, onChange }) => {
    return (
        <Stack spacing={1}>
            {platformConfigs.map((platform) => (
                <Accordion
                    key={platform.id}
                    expanded={expanded === platform.id}
                    onChange={onChange(platform.id)}
                    disableGutters
                    sx={{
                        '&:before': { display: 'none' },
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1,
                        overflow: 'hidden',
                    }}
                >
                    <AccordionSummary
                        expandIcon={<ExpandMoreIcon />}
                        sx={{
                            '& .MuiAccordionSummary-content': {
                                alignItems: 'center',
                            },
                        }}
                    >
                        <Stack direction="row" spacing={2} alignItems="center">
                            <Box
                                sx={{
                                    width: 36,
                                    height: 36,
                                    borderRadius: 1.5,
                                    bgcolor: platform.bgColor,
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'center',
                                    flexShrink: 0,
                                }}
                            >
                                {platform.icon}
                            </Box>
                            <Box>
                                <Stack direction="row" spacing={1} alignItems="center">
                                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                        {platform.name}
                                    </Typography>
                                    {platform.status === 'coming-soon' && (
                                        <Chip
                                            label="Coming Soon"
                                            size="small"
                                            sx={{
                                                height: 18,
                                                fontSize: '0.65rem',
                                                bgcolor: 'grey.100',
                                                color: 'text.secondary',
                                            }}
                                        />
                                    )}
                                    {platform.status === 'beta' && (
                                        <Chip
                                            label="Beta"
                                            size="small"
                                            color="warning"
                                            sx={{ height: 18, fontSize: '0.65rem' }}
                                        />
                                    )}
                                </Stack>
                                <Typography variant="caption" color="text.secondary">
                                    Required: {platform.requiredFields.join(', ')}
                                </Typography>
                            </Box>
                        </Stack>
                    </AccordionSummary>
                    <AccordionDetails sx={{ pt: 0, bgcolor: 'grey.50' }}>
                        {platform.steps}
                    </AccordionDetails>
                </Accordion>
            ))}
        </Stack>
    );
};

export default PlatformGuide;
