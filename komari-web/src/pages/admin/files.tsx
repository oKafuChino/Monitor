import { Suspense, useEffect, useState } from 'react';
import { Button, Flex, Heading, Select, Text, Theme } from '@radix-ui/themes';
import { Gauge } from 'lucide-react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useRPC2Call } from '@/contexts/RPC2Context';
import FileManagerPanel from '@/features/files/FileManagerPanel';
import NodeResourceMonitor from '@/features/files/NodeResourceMonitor';
import { useGuideState } from '@/components/onboarding/useGuideState';

type FileNode = {uuid:string; name:string; os:string};
export default function FilesPage() {
 const {t} = useTranslation();
 const {call} = useRPC2Call();
 const [params,setParams] = useSearchParams();
 const [nodes,setNodes] = useState<FileNode[]>([]);
 const [loading,setLoading] = useState(true);
 const [error,setError] = useState('');
 const [busy,setBusy] = useState(false);
 const [monitors,setMonitors] = useState<string[]>([]);
 const { state: guideState, mark: markGuide } = useGuideState(true);
 useEffect(() => {
  if (guideState && !guideState.workbenchOpened) markGuide('workbenchOpened');
 },[guideState,markGuide]);
 useEffect(() => {
  let disposed = false;
  call<unknown,FileNode[]>('admin:listClients').then(result => { if (!disposed) setNodes(Array.isArray(result) ? result : []); })
   .catch(err => { if (!disposed) setError(String(err)); }).finally(() => { if (!disposed) setLoading(false); });
  return () => { disposed = true; };
 },[call]);
 const requested = params.get('uuid');
 const uuid = nodes.some(node => node.uuid === requested) ? requested : null;
 return <Flex direction="column" gap="3" className="h-full min-h-0" style={{minHeight:440}}>
  <Flex align="center" justify="between" gap="3" wrap="wrap">
   <Heading size="4">{t('file_manager.title')}</Heading>
   <Flex gap="2" wrap="wrap">
    <Select.Root value={uuid || ''} disabled={loading || busy} onValueChange={value => setParams({uuid:value})}>
     <Select.Trigger aria-label={t('file_manager.select_node')} placeholder={t('file_manager.select_node')} />
     <Select.Content>{nodes.map(node => <Select.Item key={node.uuid} value={node.uuid}>{node.name || node.uuid}</Select.Item>)}</Select.Content>
    </Select.Root>
    <Button variant="soft" disabled={!uuid} onClick={() => { if (uuid) setMonitors(current => current.includes(uuid) ? current.filter(id => id !== uuid) : [...current,uuid]); }}>
     <Gauge size={16} />{t('file_manager.resource_monitor.title')}
    </Button>
   </Flex>
  </Flex>
  {error && <Text color="red">{error}</Text>}
  {!loading && requested && !uuid && <Text color="red">{t('file_manager.node_unavailable')}</Text>}
  {busy && <Text size="1" color="gray">{t('file_manager.close_before_switch')}</Text>}
  <Theme appearance="dark" className="min-h-0 flex-1 overflow-hidden rounded-lg" style={{background:'#181818'}}>
   <Suspense fallback={<Text>{t('common.loading')}</Text>}>
    <FileManagerPanel key={uuid} uuid={uuid} onBusyChange={setBusy} />
   </Suspense>
   {monitors.length > 0 && <NodeResourceMonitor clients={nodes} servers={monitors} onRemove={id => setMonitors(current => current.filter(value => value !== id))} />}
  </Theme>
 </Flex>;
}
