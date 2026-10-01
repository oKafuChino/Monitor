import fs from 'node:fs';
import path from 'node:path';

// Deterministic uncompressed ZIP; no external archiver or platform tooling.
const table=Uint32Array.from({length:256},(_,n)=>{for(let k=0;k<8;k++)n=(n&1)?0xedb88320^(n>>>1):n>>>1;return n>>>0;});
function crc32(bytes){let crc=0xffffffff;for(const b of bytes)crc=table[(crc^b)&255]^(crc>>>8);return (crc^0xffffffff)>>>0;}
export function packShare(dist, output) {
 const names=['index.html',...fs.readdirSync(path.join(dist,'assets')).sort().map(n=>'assets/'+n)];
 const local=[],central=[];let offset=0;
 for(const name of names){
  if(name!=='index.html'&&!/^assets\/[^/]+\.(js|css)$/.test(name))throw new Error(`Forbidden share asset: ${name}`);
  const bytes=fs.readFileSync(path.join(dist,name)),filename=Buffer.from(name),crc=crc32(bytes);
  const header=Buffer.alloc(30);header.writeUInt32LE(0x04034b50);header.writeUInt16LE(20,4);header.writeUInt32LE(crc,14);header.writeUInt32LE(bytes.length,18);header.writeUInt32LE(bytes.length,22);header.writeUInt16LE(filename.length,26);
  local.push(header,filename,bytes);
  const directory=Buffer.alloc(46);directory.writeUInt32LE(0x02014b50);directory.writeUInt16LE(20,4);directory.writeUInt16LE(20,6);directory.writeUInt32LE(crc,16);directory.writeUInt32LE(bytes.length,20);directory.writeUInt32LE(bytes.length,24);directory.writeUInt16LE(filename.length,28);directory.writeUInt32LE(offset,42);
  central.push(directory,filename);offset+=header.length+filename.length+bytes.length;
 }
 const directory=Buffer.concat(central),end=Buffer.alloc(22);end.writeUInt32LE(0x06054b50);end.writeUInt16LE(names.length,8);end.writeUInt16LE(names.length,10);end.writeUInt32LE(directory.length,12);end.writeUInt32LE(offset,16);
 fs.writeFileSync(output,Buffer.concat([...local,directory,end]));
}
export function readShare(archive){
 const result=new Map();let offset=0;
 while(archive.readUInt32LE(offset)===0x04034b50){
  const size=archive.readUInt32LE(offset+18),nameLength=archive.readUInt16LE(offset+26),extra=archive.readUInt16LE(offset+28);
  const name=archive.subarray(offset+30,offset+30+nameLength).toString();const start=offset+30+nameLength+extra;
  const bytes=archive.subarray(start,start+size);if(crc32(bytes)!==archive.readUInt32LE(offset+14))throw new Error('ZIP checksum mismatch');result.set(name,bytes);offset=start+size;
 }
 return result;
}
