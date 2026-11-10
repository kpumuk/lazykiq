import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { syncRubyPin } from './sync-demo-ruby.mjs';

const head = 'a'.repeat(40);
const base = 'b'.repeat(40);
const image = (version) => `FROM ruby:${version}-alpine@sha256:${'c'.repeat(64)}\n\nRUN bundle install\n`;

test('the native Ruby pin matches the demo image', () => {
  const pin = readFileSync(new URL('../../demo/.ruby-version', import.meta.url), 'utf8').trim();
  const dockerfile = readFileSync(new URL('../../demo/Dockerfile', import.meta.url), 'utf8');
  assert.ok(dockerfile.startsWith(`FROM ruby:${pin}-alpine@sha256:`),
    'Update demo/.ruby-version alongside demo/Dockerfile');
});

function fixture() {
  const pr = {
    state: 'open',
    user: { login: 'dependabot[bot]', id: 49699333, type: 'Bot' },
    base: { repo: { full_name: 'kpumuk/lazykiq' }, ref: 'main', sha: base },
    head: { repo: { full_name: 'kpumuk/lazykiq' }, ref: 'dependabot/docker/demo/ruby-4.0.10', sha: head },
    changed_files: 1,
  };
  const files = [{ filename: 'demo/Dockerfile', status: 'modified' }];
  const contents = {
    [base]: { 'demo/Dockerfile': image('4.0.9') },
    [head]: {
      'demo/Dockerfile': image('4.0.10'),
      'demo/.ruby-version': '4.0.9\n',
      'unrelated.txt': 'keep me\n',
    },
  };
  const state = { branch: head, contents: structuredClone(contents[head]), commit: null };
  let nextTree;
  const request = async (method, path, body) => {
    if (method === 'GET') {
      if (path === '/pulls/42') return structuredClone(pr);
      if (path === '/pulls/42/files?per_page=100') return structuredClone(files);
      if (path === `/git/commits/${head}`) return { tree: { sha: 'parent-tree' } };
      if (path.startsWith('/contents/')) {
        const [file, ref] = path.slice('/contents/'.length).split('?ref=');
        return { type: 'file', encoding: 'base64', content: Buffer.from(contents[ref][file]).toString('base64') };
      }
    }
    if (method === 'POST' && path === '/git/trees') {
      assert.equal(body.base_tree, 'parent-tree');
      nextTree = structuredClone(contents[head]);
      for (const entry of body.tree) nextTree[entry.path] = entry.content;
      return { sha: 'new-tree' };
    }
    if (method === 'POST' && path === '/git/commits') {
      state.commit = body;
      return { sha: 'new-commit' };
    }
    if (method === 'PATCH' && path === `/git/refs/heads/${pr.head.ref}`) {
      if (body.force || state.branch !== state.commit.parents[0]) throw new Error('Not a fast forward');
      assert.equal(body.sha, 'new-commit');
      state.branch = body.sha;
      state.contents = nextTree;
      return {};
    }
    throw new Error(`Unexpected request: ${method} ${path}`);
  };
  return { pr, files, contents, state, run: (apply = true) => syncRubyPin({ request, number: 42, head, apply }) };
}

test('a patch update appends only the native Ruby pin to the validated head', async () => {
  const f = fixture();
  const before = structuredClone(f.contents[head]);
  assert.equal((await f.run(false)).needsSync, true);
  assert.equal(f.state.commit, null);
  assert.equal((await f.run()).needsSync, true);
  assert.deepEqual(f.state.contents, { ...before, 'demo/.ruby-version': '4.0.10\n' });
  assert.equal(f.state.branch, 'new-commit');
  assert.deepEqual(f.state.commit.parents, [head]);
  assert.match(f.state.commit.message, /\[dependabot skip\]/);
});

for (const [name, change] of Object.entries({
  'already synchronized': (f) => { f.contents[head]['demo/.ruby-version'] = '4.0.10\n'; },
  'digest-only update': (f) => { f.contents[head]['demo/Dockerfile'] = image('4.0.9'); },
  'closed PR': (f) => { f.pr.state = 'closed'; },
  'human author': (f) => { f.pr.user = { login: 'kpumuk', id: 10163, type: 'User' }; },
  'spoofed bot name': (f) => { f.pr.user.id = 10163; },
  'fork': (f) => { f.pr.head.repo.full_name = 'other/lazykiq'; },
  'deleted fork': (f) => { f.pr.head.repo = null; },
  'different repository': (f) => { f.pr.base.repo.full_name = 'other/lazykiq'; },
  'different base branch': (f) => { f.pr.base.ref = 'release'; },
  'stale workflow head': (f) => { f.pr.head.sha = 'd'.repeat(40); },
  'unrelated file': (f) => {
    f.files.push({ filename: 'demo/start.rb', status: 'modified' });
    f.pr.changed_files++;
  },
  'incomplete file listing': (f) => { f.pr.changed_files = 101; },
  'no Dockerfile change': (f) => { f.files[0].filename = 'demo/.ruby-version'; },
  'renamed Dockerfile': (f) => { f.files[0].status = 'renamed'; },
  'deleted Dockerfile': (f) => { f.files[0].status = 'removed'; },
  'major upgrade': (f) => { f.contents[head]['demo/Dockerfile'] = image('5.0.0'); },
  'minor upgrade': (f) => { f.contents[head]['demo/Dockerfile'] = image('4.1.0'); },
  'image downgrade': (f) => { f.contents[head]['demo/Dockerfile'] = image('4.0.8'); },
  'native pin downgrade': (f) => { f.contents[head]['demo/.ruby-version'] = '4.0.11\n'; },
  'native release-line mismatch': (f) => { f.contents[head]['demo/.ruby-version'] = '3.4.9\n'; },
  'invalid native pin': (f) => { f.contents[head]['demo/.ruby-version'] = '4.0\n'; },
  'different image owner': (f) => { f.contents[head]['demo/Dockerfile'] = image('4.0.10').replace('ruby:', 'other/ruby:'); },
  'missing digest': (f) => { f.contents[head]['demo/Dockerfile'] = 'FROM ruby:4.0.10-alpine\n'; },
  'unrecognized baseline': (f) => { f.contents[base]['demo/Dockerfile'] = 'FROM ruby:alpine\n'; },
})) {
  test(`leaves the branch untouched: ${name}`, async () => {
    const f = fixture();
    change(f);
    const before = structuredClone(f.state);
    assert.equal((await f.run()).needsSync, false);
    assert.deepEqual(f.state, before);
  });
}

test('a previously appended pin does not prevent regeneration after a Dependabot rebase', async () => {
  const f = fixture();
  f.files.push({ filename: 'demo/.ruby-version', status: 'modified' });
  f.pr.changed_files++;
  f.contents[head]['demo/.ruby-version'] = '4.0.8\n';
  await f.run();
  assert.equal(f.state.contents['demo/.ruby-version'], '4.0.10\n');
});

test('apply rechecks eligibility after the read-only check', async () => {
  const f = fixture();
  await f.run(false);
  f.pr.head.sha = 'd'.repeat(40);
  assert.equal((await f.run()).needsSync, false);
  assert.equal(f.state.commit, null);
});

test('a concurrent branch push is not overwritten', async () => {
  const f = fixture();
  f.state.branch = 'concurrent-commit';
  const before = structuredClone(f.state.contents);
  await assert.rejects(f.run(), /Not a fast forward/);
  assert.equal(f.state.branch, 'concurrent-commit');
  assert.deepEqual(f.state.contents, before);
});
