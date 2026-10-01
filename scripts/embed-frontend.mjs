import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { zstdCompressSync, constants } from 'node:zlib';
import { packShare } from './share-archive.mjs';
const root = fileURLToPath(new URL('../',import.meta.url));
const dist = path.join(root,'komari-web','dist');
if (!fs.statSync(path.join(dist,'index.html')).isFile()) throw new Error('Build the workspace frontend first');
const out = path.join(root,'web','public','defaultTheme');
fs.mkdirSync(out,{recursive:true});
packShare(path.join(root,'komari-web','dist-share'),path.join(out,'share.zip'));
const temporary = fs.mkdtempSync(path.join(os.tmpdir(),'komari-embed-'));
const archive = path.join(temporary,'dist.tar');
try {
 const result = spawnSync('tar',['-cf',archive,'-C',dist,'.'],{stdio:'inherit'});
 if (result.status !== 0) throw new Error('tar failed');
 fs.writeFileSync(path.join(out,'dist.tar.zst'), zstdCompressSync(fs.readFileSync(archive),{params:{[constants.ZSTD_c_compressionLevel]:19}}));
 fs.copyFileSync(path.join(root,'komari-web','komari-theme.json'),path.join(out,'komari-theme.json'));
 console.log('Embedded workspace dist, including index.html, and built-in settings schema.');
} finally { if (fs.existsSync(archive)) fs.unlinkSync(archive); fs.rmdirSync(temporary); }
