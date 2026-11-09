import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const CONTRACT = 'naira.handbook/v1alpha1';

const ID_PATTERN = /^[a-z0-9]([-a-z0-9]{0,40}[a-z0-9])?$/;

const compact = (object) =>
  Object.fromEntries(Object.entries(object).filter(([, value]) => value !== undefined));

const repositoryUrl = (repository) =>
  (typeof repository === 'string' ? repository : repository?.url)
    ?.replace(/^git\+/, '')
    .replace(/\.git$/, '');

/**
 * Returns the naira-handbook.json document. The id comes from NAIRA_HANDBOOK_ID, else the package name,
 * and must be a DNS label. Fields that are not known are omitted. Throws on an invalid id.
 */
export function buildMetadata({ env, pkg, generatorVersion, now }) {
  const id = env.NAIRA_HANDBOOK_ID || pkg.name;
  if (typeof id !== 'string' || !ID_PATTERN.test(id)) {
    throw new Error(
      `invalid handbook id "${id}": use lowercase letters, digits and "-", at most 42 characters`,
    );
  }
  const source = compact({
    repository: repositoryUrl(pkg.repository),
    commit: env.GITHUB_SHA || undefined,
  });
  return compact({
    contract: CONTRACT,
    id,
    title: pkg.naira?.title || id,
    owner: pkg.naira?.owner,
    source: Object.keys(source).length > 0 ? source : undefined,
    support: typeof pkg.bugs === 'string' ? pkg.bugs : pkg.bugs?.url,
    version: pkg.version,
    builtAt: now.toISOString().replace(/\.\d{3}Z$/, 'Z'),
    generator: { name: 'vitepress', version: generatorVersion },
  });
}

function main() {
  const root = fileURLToPath(new URL('..', import.meta.url));
  const readJson = (path) => JSON.parse(readFileSync(join(root, path), 'utf8'));
  try {
    const metadata = buildMetadata({
      env: process.env,
      pkg: readJson('package.json'),
      generatorVersion: readJson('node_modules/vitepress/package.json').version,
      now: new Date(),
    });
    if (!metadata.source?.commit) {
      console.warn('GITHUB_SHA is not set: naira-handbook.json has no source commit.');
    }
    mkdirSync(join(root, 'dist'), { recursive: true });
    writeFileSync(join(root, 'dist/naira-handbook.json'), `${JSON.stringify(metadata, null, 2)}\n`);
    console.log(`wrote dist/naira-handbook.json for "${metadata.id}" ${metadata.version}`);
  } catch (error) {
    console.error(`write-metadata: ${error.message}`);
    process.exitCode = 1;
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) main();
