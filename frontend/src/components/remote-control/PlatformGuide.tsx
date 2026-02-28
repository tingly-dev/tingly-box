import { OpenInNew } from '@mui/icons-material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
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
import {
    IconBrandDingtalk,
    IconBrandDiscord,
    IconBrandSlack,
    IconBrandTelegram,
    IconBrandWechat
} from '@tabler/icons-react';

interface PlatformGuideProps {
    expanded: string | false;
    onChange: (panel: string) => (event: React.SyntheticEvent, isExpanded: boolean) => void;
}

interface PlatformConfig {
    id: string;
    name: string;
    icon: React.ReactNode;
    bgColor: string;
    iconColor: string;
    status: 'available' | 'coming-soon' | 'beta';
    requiredFields: string[];
    steps: React.ReactNode;
}

const platformConfigs: PlatformConfig[] = [
    {
        id: 'telegram',
        name: 'Telegram',
        icon: <IconBrandTelegram size={24} stroke={1.5} />,
        bgColor: '#0088cc',
        iconColor: '#ffffff',
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
        icon: <IconBrandWechat size={24} stroke={1.5} />,
        bgColor: '#07c160',
        iconColor: '#ffffff',
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
        icon: <IconBrandDiscord size={24} stroke={1.5} />,
        bgColor: '#5865F2',
        iconColor: '#ffffff',
        status: 'coming-soon',
        requiredFields: ['Bot Token', 'Message Content Intent'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                Discord bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'Lark',
        name: 'Lark',
        icon: <Typography sx={{ color: '#ffffff', fontWeight: 'bold', fontSize: '0.7rem' }}>Lark</Typography>,
        bgColor: '#3370FF',
        iconColor: '#ffffff',
        status: 'coming-soon',
        requiredFields: ['App ID', 'App Secret'],
        steps: (
            <Typography variant="body2" color="text.secondary">
                Lark bot integration is currently under development. Stay tuned for updates!
            </Typography>
        ),
    },
    {
        id: 'feishu',
        name: 'Feishu (飞书)',
        icon: <Typography sx={{ color: '#ffffff', fontWeight: 'bold', fontSize: '0.7rem' }}></Typography>,
        bgColor: '#3370FF',
        iconColor: '#ffffff',
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
        icon: <IconBrandDingtalk size={24} stroke={1.5} >钉钉</IconBrandDingtalk>,
        bgColor: '#0089FF',
        iconColor: '#ffffff',
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
        icon: <IconBrandSlack size={24} stroke={1.5} />,
        bgColor: '#4A154B',
        iconColor: '#ffffff',
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
                                    color: platform.iconColor,
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
