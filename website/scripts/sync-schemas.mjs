import { copyFile, mkdir, readdir, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const source = fileURLToPath(new URL('../../api/v1/schema/', import.meta.url));
const target = fileURLToPath(new URL('../public/schema/v1alpha1/', import.meta.url));

await rm(target, { recursive: true, force: true });
await mkdir(target, { recursive: true });

for (const entry of await readdir(source, { withFileTypes: true })) {
  if (!entry.isFile() || !entry.name.endsWith('.schema.json')) continue;
  const publicName = entry.name.replace('.schema.json', '.json');
  await copyFile(`${source}/${entry.name}`, `${target}/${publicName}`);
}
