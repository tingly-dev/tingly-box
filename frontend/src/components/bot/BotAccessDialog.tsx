import { Security } from '@/components/icons';
import { api } from '@/services/api';
import type {BotCapability, BotGroupDetail, BotSettings, DirectChatDetail, CapabilityName} from '@/types/bot';
import {
    Alert, Box, Button, Chip, CircularProgress, Dialog, DialogActions, DialogContent,
    DialogTitle, Divider, FormControlLabel, Stack, Switch, TextField, Typography,
} from '@mui/material';
import {useCallback, useEffect, useState} from 'react';
import PairingCodePanel from './PairingCodePanel';

interface Props { open:boolean; bot:BotSettings|null; onClose:()=>void; onChanged:()=>void; }

const permissionAllowed = (chat:DirectChatDetail, action:string) =>
    chat.permissions.some((permission) => permission.action === action && permission.effect === 'allow');

const BotAccessDialog = ({open, bot, onClose, onChanged}:Props) => {
    const [capabilities,setCapabilities]=useState<BotCapability[]>([]);
    const [chats,setChats]=useState<DirectChatDetail[]>([]);
    const [groups,setGroups]=useState<BotGroupDetail[]>([]);
    const [loading,setLoading]=useState(false);
    const [saving,setSaving]=useState(false);
    const [error,setError]=useState('');
	const [actorGroup,setActorGroup]=useState<BotGroupDetail|null>(null);
	const [externalActorID,setExternalActorID]=useState('');
	const [actorName,setActorName]=useState('');

    const load=useCallback(async()=>{
        if(!bot?.uuid)return;
        setLoading(true);setError('');
        try{
            const [capabilityData,chatData,groupData]=await Promise.all([
                api.listBotCapabilities(bot.uuid),api.listBotDirectChats(bot.uuid),api.listBotGroups(bot.uuid),
            ]);
            const details=await Promise.all((groupData.groups||[]).map((group:any)=>api.getBotGroup(bot.uuid!,group.id)));
            setCapabilities(capabilityData.capabilities||[]);setChats(chatData.chats||[]);setGroups(details);
        }catch(e){setError((e as Error).message)}finally{setLoading(false)}
    },[bot?.uuid]);

    useEffect(()=>{if(open)void load()},[open,load]);
    const mutate=async(action:()=>Promise<unknown>)=>{setSaving(true);setError('');try{await action();await load();onChanged()}catch(e){setError((e as Error).message)}finally{setSaving(false)}};
    const capabilityOn=(name:CapabilityName)=>capabilities.find((item)=>item.capability===name)?.enabled===true;
    const setPreset=(chat:DirectChatDetail,preset:'full'|'notify')=>mutate(async()=>{
        // One atomic batch write: a partial network failure must never leave
        // mixed rows (e.g. start=allow with approve=deny silently breaking
        // permission replies).
        const rows:Array<{capability:string;action:string;effect:'allow'|'deny'}>=[];
        const put=(capability:string,action:string,allow:boolean)=>rows.push({capability,action,effect:allow?'allow':'deny'});
        put('notify','access',true);put('notify','notify.receive',true);put('notify','notify.reply',true);
        put('remote_control','access',preset==='full');put('remote_control','remote_control.start',preset==='full');put('remote_control','remote_control.approve',preset==='full');put('remote_control','remote_control.privileged',false);
        await api.setBotDirectChatPermissions(bot!.uuid!,chat.chat.id,rows);
    });

    return <Dialog open={open} onClose={onClose} fullWidth maxWidth="md">
        <DialogTitle sx={{display:'flex',alignItems:'center',gap:1}}><Security color="primary"/> {bot?.name||bot?.platform} access</DialogTitle>
        <DialogContent dividers>
            <Stack spacing={3}>
                <Box>
                    <Typography variant="body2" color="text.secondary">Bot UUID</Typography>
                    <Typography component="code" sx={{fontFamily:'monospace',wordBreak:'break-all'}}>{bot?.uuid}</Typography>
                    {bot && <PairingCodePanel bot={bot}/>}
                </Box>
                {error&&<Alert severity="error">{error}</Alert>}
                {loading?<Box sx={{display:'flex',justifyContent:'center',py:6}}><CircularProgress/></Box>:<>
                    <Box>
                        <Typography variant="h6">Capabilities</Typography>
                        <Typography variant="body2" color="text.secondary" sx={{mb:1}}>What this Bot can provide. Turning off the last capability stops the connection without deleting its configuration.</Typography>
                        <Stack direction={{xs:'column',sm:'row'}} spacing={3}>
                            {(['remote_control','notify'] as CapabilityName[]).map((name)=><FormControlLabel key={name} control={<Switch checked={capabilityOn(name)} disabled={saving} onChange={(_,enabled)=>void mutate(()=>api.setBotCapability(bot!.uuid!,name,enabled))}/>} label={name==='remote_control'?'Remote Control':'Notify'}/>) }
                        </Stack>
                    </Box>
                    <Divider/>
                    <Box>
                        <Typography variant="h6">Direct Chats</Typography>
                        <Typography variant="body2" color="text.secondary" sx={{mb:1}}>Who is paired, what they can do, and the concrete platform chat ID.</Typography>
                        {chats.length===0?<Alert severity="info">No Direct Chats yet. Send the Bot <code>/bind &lt;pairing code&gt;</code> in a direct message, then return here.</Alert>:<Stack spacing={1.5}>{chats.map((chat)=><Box key={chat.chat.id} sx={{p:1.5,border:1,borderColor:'divider',borderRadius:1.5}}>
                            <Stack direction={{xs:'column',sm:'row'}} sx={{justifyContent:'space-between',gap:1}}>
                                <Box><Typography sx={{fontWeight:600}}>{chat.chat.peer_actor_id?'Paired person':'Unpaired chat'}</Typography><Typography variant="caption" sx={{fontFamily:'monospace',color:'text.primary'}}>{chat.chat.external_chat_id}</Typography></Box>
                                <Stack direction="row" spacing={1} sx={{flexWrap:'wrap'}}>{(()=>{
                                    // Show the real state of both actions the remote-control
                                    // flow depends on: start (launch runs) AND approve (answer
                                    // permission/question prompts). A mixed state silently
                                    // breaks prompt replies, so it must be loud here, with the
                                    // concrete per-action values in the tooltip.
                                    const start=permissionAllowed(chat,'remote_control.start');
                                    const approve=permissionAllowed(chat,'remote_control.approve');
                                    if(start&&approve)return <Chip size="small" label="Remote Control" color="primary"/>;
                                    if(!start&&!approve)return <Chip size="small" label="No Remote Control"/>;
                                    return <Chip size="small" color="warning" label={`Remote Control broken: ${start?'approve denied':'start denied'}`} title="start launches runs; approve answers permission/question prompts. Re-apply the Full preset to repair."/>;
                                })()}<Chip size="small" label={permissionAllowed(chat,'notify.receive')?'Notify':'No Notify'}/><FormControlLabel sx={{m:0}} control={<Switch size="small" checked={chat.chat.blocked} onChange={(_,blocked)=>void mutate(()=>api.setBotDirectChatBlocked(bot!.uuid!,chat.chat.id,blocked))}/>} label="Blocked"/></Stack>
                            </Stack>
                            <Stack direction="row" spacing={1} sx={{mt:1}}><Button size="small" disabled={saving||!chat.chat.peer_actor_id} onClick={()=>void setPreset(chat,'full')}>Full access</Button><Button size="small" disabled={saving} onClick={()=>void setPreset(chat,'notify')}>Notify only</Button></Stack>
                        </Box>)}</Stack>}
                    </Box>
                    <Divider/>
                    <Box>
                        <Typography variant="h6">Groups</Typography>
                        <Typography variant="body2" color="text.secondary" sx={{mb:1}}>Group capability access and authorized Actors are separate controls.</Typography>
                        {groups.length===0?<Alert severity="info">No Groups observed yet. Add the Bot to a group and send it a message; new groups start with no access.</Alert>:<Stack spacing={1.5}>{groups.map(({group,capabilities:groupCaps,actors})=><Box key={group.id} sx={{p:1.5,border:1,borderColor:'divider',borderRadius:1.5}}>
                            <Stack direction={{xs:'column',sm:'row'}} sx={{justifyContent:'space-between',gap:1}}><Box><Typography sx={{fontWeight:600}}>{group.name||'Group'}</Typography><Typography variant="caption" sx={{fontFamily:'monospace',color:'text.primary'}}>{group.external_group_id}</Typography></Box><Stack direction="row"><FormControlLabel control={<Switch size="small" checked={groupCaps.notify==='allow'} onChange={(_,enabled)=>void mutate(()=>api.setBotGroupCapability(bot!.uuid!,group.id,'notify',enabled?'allow':'deny'))}/>} label="Notify"/><FormControlLabel control={<Switch size="small" checked={groupCaps.remote_control==='allow'} onChange={(_,enabled)=>void mutate(()=>api.setBotGroupCapability(bot!.uuid!,group.id,'remote_control',enabled?'allow':'deny'))}/>} label="Remote Control"/></Stack></Stack>
                            <Stack direction="row" sx={{alignItems:'center',gap:1,mt:0.5,flexWrap:'wrap'}}><Typography variant="body2" color={groupCaps.remote_control==='allow'&&actors.length===0?'warning.main':'text.secondary'}>{actors.length} authorized actor{actors.length===1?'':'s'}{groupCaps.remote_control==='allow'&&actors.length===0?' — nobody can control this Bot yet.':''}</Typography>{actors.map(({actor})=><Chip key={actor.id} size="small" label={actor.display_name||actor.external_actor_id}/>) }<Button size="small" onClick={()=>{setActorGroup({group,capabilities:groupCaps,actors});setExternalActorID('');setActorName('')}}>Add actor</Button></Stack>
                        </Box>)}</Stack>}
                    </Box>
                </>}
            </Stack>
        </DialogContent>
        <DialogActions><Button onClick={onClose}>Close</Button></DialogActions>
		<Dialog open={Boolean(actorGroup)} onClose={()=>setActorGroup(null)} fullWidth maxWidth="xs"><DialogTitle>Add authorized actor</DialogTitle><DialogContent><Stack spacing={2} sx={{pt:1}}><Alert severity="info">This grants Start and Approve in this Group. Privileged access stays denied.</Alert><TextField autoFocus label="Platform actor ID" value={externalActorID} onChange={(event)=>setExternalActorID(event.target.value)} helperText="Use the concrete user ID reported by the IM platform."/><TextField label="Display name (optional)" value={actorName} onChange={(event)=>setActorName(event.target.value)}/></Stack></DialogContent><DialogActions><Button onClick={()=>setActorGroup(null)}>Cancel</Button><Button variant="contained" disabled={!externalActorID.trim()||saving} onClick={()=>void mutate(async()=>{await api.addBotGroupActor(bot!.uuid!,actorGroup!.group.id,externalActorID.trim(),externalActorID.trim(),actorName.trim()||undefined);setActorGroup(null)})}>Add controller</Button></DialogActions></Dialog>
    </Dialog>;
};

export default BotAccessDialog;
