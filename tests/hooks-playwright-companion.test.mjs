import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { existsSync, readdirSync } from 'node:fs';
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, symlink, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { promisify } from 'node:util';
import { buildPreparationPlan, createDependencyRunner, prepareDependencies } from '../hooks/lib/dependencies.mjs';
import { CATALOG_BY_ID } from '../hooks/lib/dependency-catalog.mjs';
import { createEventBudget } from '../hooks/lib/budget.mjs';
import { mutateSetup } from '../hooks/lib/settings.mjs';

// Labelled static package layout and fake command: no upstream CLI code is executed.
async function fixture(t, host = 'codex', scope = 'user') {
  const root = await mkdtemp(path.join(os.tmpdir(), 'agent-team-companion-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const projectRoot = path.join(root, 'project');
  const selectedRoot = scope === 'user' ? path.join(root, 'home') : projectRoot;
  const paths = { projectRoot, toolRoot: path.join(selectedRoot, '.agent-team/tools'), skillRoot: path.join(selectedRoot, host === 'codex' ? '.agents/skills' : '.claude/skills') };
  const packageRoot = path.join(paths.toolRoot, process.platform === 'win32' ? 'node_modules' : 'lib/node_modules', '@playwright/cli');
  const source = path.join(packageRoot, 'skills/playwright-cli');
  await mkdir(projectRoot, { recursive: true });
  await mkdir(path.join(source, 'references'), { recursive: true });
  await writeFile(path.join(projectRoot, '.gitignore'), '# preserve user ignore\n');
  await writeFile(path.join(packageRoot, 'package.json'), JSON.stringify({ name: '@playwright/cli', version: '0.1.19' }));
  await writeFile(path.join(source, 'SKILL.md'), '# Labelled static Playwright companion fixture\n[Reference](references/example.md)\n');
  await writeFile(path.join(source, 'references/example.md'), 'Complete companion reference\n');
  const executable = path.join(root, 'labelled-fake-playwright-command.mjs');
  const log = path.join(root, 'fake-command-log.jsonl');
  await writeFile(executable, `#!/usr/bin/env node\nimport { appendFileSync } from 'node:fs';\nconst args = process.argv.slice(2);\nappendFileSync(${JSON.stringify(log)}, JSON.stringify({ args, browsers: process.env.PLAYWRIGHT_BROWSERS_PATH, notifier: process.env.NO_UPDATE_NOTIFIER }) + '\\n');\nif (args[0] === '--version') console.log('0.1.19');\nelse if (args[0] !== 'install-browser' || args[1] !== 'chromium') process.exit(91);\n`);
  await chmod(executable, 0o755);
  const dependency = { ...CATALOG_BY_ID.get('playwright-cli'), executable };
  const runner = createDependencyRunner({ host, scope, paths });
  return { root, paths, source, packageRoot, dependency, runner, log, destination: path.join(paths.skillRoot, 'playwright-cli') };
}

test('Playwright plans never invoke upstream workspace/skill installation in either scope', () => {
  for (const scope of ['user', 'project']) for (const host of ['codex', 'claude-code']) {
    const paths = { projectRoot: '/fixture/project', toolRoot: '/fixture/selected/tools', skillRoot: '/fixture/selected/skills' };
    const plan = buildPreparationPlan({ dependencyId: 'playwright-cli', host, scope, paths });
    assert.equal(plan.install.length, 1);
    assert.equal(plan.install[0].file, 'npm');
    assert.deepEqual(plan.browserInstall.args, ['install-browser', 'chromium']);
    assert.equal(plan.browserInstall.env.PLAYWRIGHT_BROWSERS_PATH, '/fixture/selected/tools/playwright-browsers');
  }
});

test('complete pinned companion goes only to the selected host and scope with scoped browser acquisition', async (t) => {
  for (const scope of ['user', 'project']) for (const host of ['codex', 'claude-code']) {
    const f = await fixture(t, host, scope);
    const result = await f.runner({ dependency: f.dependency, phase: 'companion' });
    assert.equal(result.status, 'passed', result.evidence);
    assert.equal(await readFile(path.join(f.destination, 'references/example.md'), 'utf8'), 'Complete companion reference\n');
    const metadata = JSON.parse(await readFile(path.join(f.destination, '.agent-team-source.json'), 'utf8'));
    assert.equal(metadata.revision, '0.1.19');
    assert.equal(await readFile(path.join(f.paths.projectRoot, '.gitignore'), 'utf8'), '# preserve user ignore\n');
    await assert.rejects(readFile(path.join(f.paths.projectRoot, '.playwright/cli.config.json')), { code: 'ENOENT' });
    if (scope === 'user') assert.deepEqual(await readdir(f.paths.projectRoot), ['.gitignore']);
    const calls = (await readFile(f.log, 'utf8')).trim().split('\n').map(JSON.parse);
    assert.deepEqual(calls.map(({ args }) => args), [['install-browser', 'chromium']]);
    assert.equal(calls[0].browsers, path.join(f.paths.toolRoot, 'playwright-browsers'));
    assert.equal(calls[0].notifier, '1');
  }
});

test('unmanaged and subsequently edited companions are preserved before any browser command', async (t) => {
  const f = await fixture(t);
  await mkdir(f.destination, { recursive: true });
  const skill = path.join(f.destination, 'SKILL.md');
  await writeFile(skill, '# Custom skill\n');
  const custom = await f.runner({ dependency: f.dependency, phase: 'companion' });
  assert.equal(custom.status, 'customized');
  assert.equal(await readFile(skill, 'utf8'), '# Custom skill\n');
  await assert.rejects(readFile(f.log), { code: 'ENOENT' });

  const managed = await fixture(t);
  assert.equal((await managed.runner({ dependency: managed.dependency, phase: 'companion' })).status, 'passed');
  await writeFile(path.join(managed.destination, 'references/example.md'), 'User edit\n');
  const before = await readFile(managed.log, 'utf8');
  const edited = await managed.runner({ dependency: managed.dependency, phase: 'companion' });
  assert.equal(edited.status, 'customized');
  assert.equal(await readFile(path.join(managed.destination, 'references/example.md'), 'utf8'), 'User edit\n');
  assert.equal(await readFile(managed.log, 'utf8'), before);
});

test('compatible executable reuse prepares the missing companion before functional and worker checks', async (t) => {
  const f = await fixture(t);
  const setupPath = path.join(f.root, 'setup.json');
  await writeFile(setupPath, JSON.stringify({ skill: 'agent-team', projectId: 'p', version: 1, tracker: { kind: 'markdown', path: 'TASKS.md' } }));
  const phases = [];
  const result = await prepareDependencies({
    setupPath, expectedVersion: 1, operationId: 'reuse', writer: { id: 'owner', role: 'project_orchestrator' },
    loadRegistry: async () => ({ projectOwner: 'owner' }), host: 'codex', scope: 'user', paths: f.paths, selections: { defaults: [] },
    runner: async ({ dependency, phase }) => {
      if (dependency.id !== 'playwright-cli') return { status: 'passed', version: dependency.version };
      phases.push(phase);
      if (phase === 'probe' || phase === 'companion') return f.runner({ dependency: f.dependency, phase });
      assert.notEqual(phase, 'install', 'compatible tool must not be reinstalled');
      assert.equal(await readFile(path.join(f.destination, 'references/example.md'), 'utf8'), 'Complete companion reference\n');
      return phase === 'worker' ? { status: 'unverified', evidence: 'No native host was exercised.' } : { status: 'passed' };
    },
  });
  assert.deepEqual(phases, ['probe', 'companion', 'functional', 'worker']);
  const receipt = result.receipts.find(({ id }) => id === 'playwright-cli');
  assert.equal(receipt.installed, 'reused');
  assert.equal(receipt.availableToWorker, 'unknown');
});

test('incomplete or wrong-version package companions fail before copying or browser commands', async (t) => {
  for (const defect of ['reference', 'version', 'symlink']) {
    const f = await fixture(t);
    if (defect === 'reference') await rm(path.join(f.source, 'references/example.md'));
    if (defect === 'version') await writeFile(path.join(f.packageRoot, 'package.json'), '{"name":"@playwright/cli","version":"0.0.1"}');
    if (defect === 'symlink') await symlink(f.paths.projectRoot, path.join(f.source, 'escape'));
    const result = await f.runner({ dependency: f.dependency, phase: 'companion' });
    assert.notEqual(result.status, 'passed');
    await assert.rejects(readFile(path.join(f.destination, 'SKILL.md')), { code: 'ENOENT' });
    await assert.rejects(readFile(f.log), { code: 'ENOENT' });
  }
});

test('companion verification ignores example snapshot links inside fenced code', async (t) => {
  const f = await fixture(t);
  await writeFile(path.join(f.source, 'SKILL.md'), '# Companion\n```bash\n[Snapshot](.playwright-cli/example.yml)\n```\n[Reference](references/example.md)\n');
  const result = await f.runner({ dependency: f.dependency, phase: 'companion' });
  assert.equal(result.status, 'passed', result.evidence);
});

test('failed or customized companion gates block functional and fresh-worker checks on reuse', async (t) => {
  for (const status of ['failed', 'customized']) {
    const f = await fixture(t);
    const setupPath = path.join(f.root, 'setup.json');
    await writeFile(setupPath, JSON.stringify({ skill: 'agent-team', projectId: 'p', version: 1, tracker: { kind: 'markdown', path: 'TASKS.md' } }));
    const phases = [];
    const result = await prepareDependencies({
      setupPath, expectedVersion: 1, operationId: 'blocked-companion', writer: { id: 'owner', role: 'project_orchestrator' },
      loadRegistry: async () => ({ projectOwner: 'owner' }), host: 'codex', scope: 'user', paths: f.paths, selections: { defaults: [] },
      runner: async ({ dependency, phase }) => {
        if (dependency.id === 'playwright-cli') {
          phases.push(phase);
          if (phase === 'companion') return { status, evidence: 'Selected companion is unavailable.' };
        }
        return { status: 'passed', version: dependency.version };
      },
    });
    assert.deepEqual(phases, ['probe', 'companion']);
    const receipt = result.receipts.find(({ id }) => id === 'playwright-cli');
    assert.equal(receipt.status, status === 'customized' ? 'cannot_use' : 'failed');
    assert.equal(receipt.functional, 'not_run');
    assert.equal(receipt.availableToWorker, 'not_run');
  }
});

test('actual integrity-pinned npm companion copies completely without executing package code', { skip: process.env.AGENT_TEAM_TEST_PINNED_PLAYWRIGHT_PACKAGE !== '1' }, async (t) => {
  const f = await fixture(t);
  const response = await fetch('https://registry.npmjs.org/@playwright/cli/-/cli-0.1.19.tgz');
  assert.equal(response.ok, true);
  const archive = Buffer.from(await response.arrayBuffer());
  assert.equal(createHash('sha512').update(archive).digest('base64'), 'eGXIsYa5D+dC6wHGf+9uEislhPGip1djK+yiNAD7BVsXN3WzzR1J4ClFAhYhyu7wSEFqhcPrqXAYeBJF1dKJ7A==');
  const archivePath = path.join(f.root, 'pinned-cli.tgz');
  await writeFile(archivePath, archive);
  await rm(f.packageRoot, { recursive: true });
  await mkdir(f.packageRoot, { recursive: true });
  await promisify(execFile)('tar', ['-xzf', archivePath, '--strip-components=1', '-C', f.packageRoot]);
  const result = await f.runner({ dependency: f.dependency, phase: 'companion' });
  assert.equal(result.status, 'passed', result.evidence);
  assert.equal(await readFile(path.join(f.destination, 'SKILL.md'), 'utf8'), await readFile(path.join(f.source, 'SKILL.md'), 'utf8'));
  assert.equal((await readdir(path.join(f.destination, 'references'))).length, 9);
  for (const file of await readdir(path.join(f.source, 'references'))) {
    assert.deepEqual(await readFile(path.join(f.destination, 'references', file)), await readFile(path.join(f.source, 'references', file)));
  }
});

test('companion filesystem byte and entry limits preserve oversized customized resources', async (t) => {
  for (const defect of ['bytes', 'entries']) {
    const f = await fixture(t);
    if (defect === 'bytes') await writeFile(path.join(f.source, 'oversized.bin'), Buffer.alloc(3 * 1024 * 1024));
    else for (let i = 0; i < 300; i++) await writeFile(path.join(f.source, `entry-${i}`), 'fixture');
    const result = await f.runner({ dependency: f.dependency, phase: 'companion' });
    assert.equal(result.status, 'failed');
    assert.match(result.evidence, /limit/i);
    await assert.rejects(readFile(f.log), { code: 'ENOENT' });
    await assert.rejects(readFile(path.join(f.destination, 'SKILL.md')), { code: 'ENOENT' });
  }
});

test('companion filesystem deadline interrupts validation before copying or browser commands', async (t) => {
  const f = await fixture(t);
  let checks = 0;
  const budget = { signal: new AbortController().signal, check() {
    if (++checks >= 8) throw Object.assign(new Error('Fixture filesystem deadline'), { code: 'EVENT_DEADLINE' });
  } };
  await assert.rejects(f.runner({ dependency: f.dependency, phase: 'companion', budget }), { code: 'EVENT_DEADLINE' });
  assert.equal(checks, 8);
  await assert.rejects(readFile(f.log), { code: 'ENOENT' });
  await assert.rejects(readFile(path.join(f.destination, 'SKILL.md')), { code: 'ENOENT' });
});

test('shared browser deadline kills the fake child and never commits a late receipt', async (t) => {
  const f = await fixture(t);
  const late = path.join(f.root, 'late-browser-success');
  await writeFile(f.dependency.executable, `#!/usr/bin/env node\nimport { writeFileSync } from 'node:fs';\nif (process.argv[2] === '--version') console.log('0.1.19');\nelse setTimeout(() => writeFileSync(${JSON.stringify(late)}, 'late'), 700);\n`);
  const setupPath = path.join(f.root, 'setup.json');
  const original = JSON.stringify({ skill: 'agent-team', projectId: 'p', version: 1, tracker: { kind: 'markdown', path: 'TASKS.md' } });
  await writeFile(setupPath, original);
  const budget = createEventBudget(250);
  try {
    await assert.rejects(prepareDependencies({
      setupPath, expectedVersion: 1, operationId: 'browser-deadline', writer: { id: 'owner', role: 'project_orchestrator' },
      loadRegistry: async () => ({ projectOwner: 'owner' }), host: 'codex', scope: 'user', paths: f.paths, selections: { defaults: [] }, budget,
      runner: async (request) => request.dependency.id === 'playwright-cli'
        ? f.runner({ ...request, dependency: f.dependency }) : { status: 'passed', version: request.dependency.version },
    }), { code: 'EVENT_DEADLINE' });
    assert.equal(await readFile(setupPath, 'utf8'), original);
    await new Promise(resolve => setTimeout(resolve, 800));
    await assert.rejects(readFile(late), { code: 'ENOENT' });
    await assert.rejects(readdir(path.join(f.root, '.locks/setup.lock')), { code: 'ENOENT' });
  } finally { budget.close(); }
});

test('setup commit edge rejects an expired mutation even when its callback returns success', async (t) => {
  const f = await fixture(t);
  const setupPath = path.join(f.root, 'setup.json');
  const original = '{"skill":"agent-team","projectId":"p","version":1}';
  await writeFile(setupPath, original);
  const budget = createEventBudget(30);
  try {
    await assert.rejects(mutateSetup({
      setupPath, expectedVersion: 1, operationId: 'late-mutation', operation: { kind: 'fixture' },
      writer: { id: 'owner', role: 'project_orchestrator' }, loadRegistry: async () => ({ projectOwner: 'owner' }), budget,
      mutate: async setup => { await new Promise(resolve => setTimeout(resolve, 70)); return { setup }; },
    }), { code: 'EVENT_DEADLINE' });
    assert.equal(await readFile(setupPath, 'utf8'), original);
  } finally { budget.close(); }
});

test('destination validation limits preserve oversized existing managed companions', async (t) => {
  for (const defect of ['bytes', 'entries']) {
    const f = await fixture(t);
    assert.equal((await f.runner({ dependency: f.dependency, phase: 'companion' })).status, 'passed');
    const before = await readFile(f.log, 'utf8');
    if (defect === 'bytes') await writeFile(path.join(f.destination, 'custom.bin'), Buffer.alloc(3 * 1024 * 1024, 7));
    else for (let i = 0; i < 300; i++) await writeFile(path.join(f.destination, `custom-${i}`), 'preserve');
    const result = await f.runner({ dependency: f.dependency, phase: 'companion' });
    assert.equal(result.status, 'customized');
    assert.match(result.evidence, /limit/i);
    assert.equal(await readFile(f.log, 'utf8'), before);
    if (defect === 'bytes') assert.deepEqual(await readFile(path.join(f.destination, 'custom.bin')), Buffer.alloc(3 * 1024 * 1024, 7));
    else assert.equal((await readdir(f.destination)).filter(name => name.startsWith('custom-')).length, 300);
  }
});

test('filesystem deadline during companion copy stops before later files and provenance', async (t) => {
  const f = await fixture(t);
  await writeFile(path.join(f.source, '00-first.txt'), 'First bounded file\n');
  const firstFile = path.join(f.destination, '00-first.txt');
  const budget = { signal: new AbortController().signal, check() {
    if (existsSync(firstFile)) throw Object.assign(new Error('Fixture copy deadline'), { code: 'EVENT_DEADLINE' });
  } };
  await assert.rejects(f.runner({ dependency: f.dependency, phase: 'companion', budget }), { code: 'EVENT_DEADLINE' });
  assert.equal(await readFile(firstFile, 'utf8'), 'First bounded file\n');
  await assert.rejects(readFile(path.join(f.destination, 'SKILL.md')), { code: 'ENOENT' });
  await assert.rejects(readFile(path.join(f.destination, 'references/example.md')), { code: 'ENOENT' });
  await assert.rejects(readFile(path.join(f.destination, '.agent-team-source.json')), { code: 'ENOENT' });
  await assert.rejects(readFile(f.log), { code: 'ENOENT' });
});

test('setup pre-rename deadline preserves original setup and removes its temporary file', async (t) => {
  const f = await fixture(t);
  const setupPath = path.join(f.root, 'setup.json');
  const original = '{"skill":"agent-team","projectId":"p","version":1}';
  await writeFile(setupPath, original);
  const budget = { signal: new AbortController().signal, remaining: () => 1000, check() {
    if (readdirSync(f.root).some(name => name.startsWith('setup.json.') && name.endsWith('.tmp'))) {
      throw Object.assign(new Error('Fixture pre-rename deadline'), { code: 'EVENT_DEADLINE' });
    }
  } };
  await assert.rejects(mutateSetup({
    setupPath, expectedVersion: 1, operationId: 'pre-rename', operation: { kind: 'fixture' },
    writer: { id: 'owner', role: 'project_orchestrator' }, loadRegistry: async () => ({ projectOwner: 'owner' }), budget,
    mutate: async setup => ({ setup }),
  }), { code: 'EVENT_DEADLINE' });
  assert.equal(await readFile(setupPath, 'utf8'), original);
  assert.equal((await readdir(f.root)).some(name => name.endsWith('.tmp')), false);
});
