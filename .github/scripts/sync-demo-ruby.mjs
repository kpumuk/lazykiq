import { appendFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const repository = 'kpumuk/lazykiq';
const dockerfile = 'demo/Dockerfile';
const rubyPin = 'demo/.ruby-version';

// PR contents are data, never executable code. Writes have an explicit parent
// and use a non-forced ref update so a concurrent push cannot be overwritten.
export async function syncRubyPin({ request, number, head, apply = false }) {
  const skip = (reason) => ({ needsSync: false, reason });
  const pr = await request('GET', `/pulls/${number}`);
  // dependabot[bot] account ID 49699333: https://api.github.com/users/dependabot%5Bbot%5D
  if (pr.state !== 'open' || pr.user.login !== 'dependabot[bot]' ||
      pr.user.id !== 49699333 || pr.user.type !== 'Bot' ||
      pr.base.repo.full_name !== repository || pr.head.repo?.full_name !== repository ||
      pr.base.ref !== 'main' || pr.head.sha !== head) {
    return skip('Not a current, same-repository Dependabot PR against main');
  }

  const files = await request('GET', `/pulls/${number}/files?per_page=100`);
  if (files.length !== pr.changed_files || files.length > 2 ||
      !files.some((file) => file.filename === dockerfile) ||
      files.some((file) => file.status !== 'modified' ||
        ![dockerfile, rubyPin].includes(file.filename))) {
    return skip('Not an isolated demo Ruby image update');
  }

  const read = async (path, ref) => {
    const file = await request('GET', `/contents/${path}?ref=${ref}`);
    if (file.type !== 'file' || file.encoding !== 'base64') {
      throw new Error(`Expected a regular file: ${path}`);
    }
    return Buffer.from(file.content, 'base64').toString('utf8');
  };
  const [before, after, pin] = await Promise.all([
    read(dockerfile, pr.base.sha), read(dockerfile, head), read(rubyPin, head),
  ]);
  const imageVersion = (source) => source.match(
    /^FROM ruby:([0-9]+\.[0-9]+\.[0-9]+)-alpine@sha256:[a-f0-9]{64}\r?\n/,
  )?.[1];
  const previous = imageVersion(before);
  const target = imageVersion(after);
  const current = pin.trim();
  if (!previous || !target || !/^[0-9]+\.[0-9]+\.[0-9]+$/.test(current)) {
    return skip('Expected exact Ruby patch versions and a digest-pinned official Alpine image');
  }
  const releaseLine = (version) => version.split('.').slice(0, 2).join('.');
  const patch = (version) => Number(version.split('.')[2]);
  if ([previous, current].some((version) =>
    releaseLine(version) !== releaseLine(target) || patch(version) > patch(target))) {
    return skip('Refusing a Ruby release-line change or downgrade');
  }
  if (current === target) return skip('Ruby pin already matches the image');
  if (!apply) return { needsSync: true, reason: `Ruby pin needs ${target}` };

  const parent = await request('GET', `/git/commits/${head}`);
  const tree = await request('POST', '/git/trees', {
    base_tree: parent.tree.sha,
    tree: [{ path: rubyPin, mode: '100644', type: 'blob', content: `${target}\n` }],
  });
  const commit = await request('POST', '/git/commits', {
    message: `chore: Sync demo Ruby to ${target} [dependabot skip]`,
    tree: tree.sha,
    parents: [head],
  });
  const branch = pr.head.ref.split('/').map(encodeURIComponent).join('/');
  await request('PATCH', `/git/refs/heads/${branch}`, {
    sha: commit.sha,
    force: false,
  });
  return { needsSync: true, reason: `Synced Ruby pin to ${target}` };
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { PR_NUMBER: number, PR_HEAD_SHA: head, GH_TOKEN: token } = process.env;
  if (!/^[1-9][0-9]*$/.test(number ?? '') || !/^[a-f0-9]{40}$/.test(head ?? '') || !token) {
    throw new Error('PR_NUMBER, PR_HEAD_SHA, and GH_TOKEN are required');
  }
  const request = async (method, path, body) => {
    const response = await fetch(`https://api.github.com/repos/${repository}${path}`, {
      method,
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: 'application/vnd.github+json',
        'Content-Type': 'application/json',
        'X-GitHub-Api-Version': '2022-11-28',
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) throw new Error(`GitHub API ${method} ${path}: ${response.status}`);
    return response.json();
  };
  const result = await syncRubyPin({ request, number, head, apply: process.argv.includes('--apply') });
  console.log(result.reason);
  if (process.env.GITHUB_OUTPUT) {
    appendFileSync(process.env.GITHUB_OUTPUT, `needs-sync=${result.needsSync}\n`);
  }
}
