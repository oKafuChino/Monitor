import { Navigate, useSearchParams } from 'react-router-dom';
export default function LegacyTerminal() {
 const [params] = useSearchParams();
 const uuid = params.get('uuid');
 return <Navigate replace to={'/admin/files' + (uuid ? '?uuid=' + encodeURIComponent(uuid) : '')} />;
}
