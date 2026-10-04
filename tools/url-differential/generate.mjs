#!/usr/bin/env node
// Reference-only development harness. No npm dependencies; runtime code never
// calls Node. Keep this seeded corpus separate from the normal Go test suite.
const profiles = {
  '24.19.0': { ada: '3.4.4', icu: '78.3', unicode: '17.0' },
  '24.21.0': { ada: '4.0.0', icu: '78.3', unicode: '17.0' },
};
const profile = profiles[process.versions.node];
if (!profile || Object.entries(profile).some(([key, value]) => process.versions[key] !== value)) {
  throw new Error(`Unexpected Node/ADA/ICU/Unicode reference build: ${JSON.stringify(process.versions)}`);
}

const initialSeed = 43807;
let seed = initialSeed;
function random(limit) {
  seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
  return seed % limit;
}
function pick(values) { return values[random(values.length)]; }
const hosts = ['example.com', '127.1', '0x7f.1', '0177.0.1', '0x', 'example.123', '09', '[::1]', '[::ffff:192.0.2.1]', 'exa_mple.test', 'foo..test', 'xn--9ca.test', '.', '0x.', '%65xample.com', 'foo!bar.test'];
const users = ['', 'u:p@', 'u:@', ':p@', ':@', 'u@x:p@', 'a:b:c@', '[];|@', 'u%3A:p%40@'];
const ports = ['', ':', ':80', ':443', ':8080', ':00080', ':65535', ':65536', ':-1', ':0'];
const paths = ['', '/', '/a', '/a/../b', '/a/%2e/', '/a/.%2E', '/a b/é', '\\a\\b', '//a//b', '/%00', '/%', '/%2f', '/a{}^`<b>'];
const queries = ['', '?', '?a=b+c', '?x=é', "?x='foo'", '?x=%GG', '?x=a b', '?x=\\'];
const cases = [];
for (let i = 0; i < 10000; i++) {
  const input = pick(['http:', 'https:', 'ftp:', 'ws:', 'wss:']) + pick(['//', '/', '///', '\\\\']) + pick(users) + pick(hosts) + pick(ports) + pick(paths) + pick(queries);
  let expected = null;
  try { expected = new URL(input).href; } catch { /* Invalid inputs must fail. */ }
  cases.push({ input, expected });
}
console.log(JSON.stringify({
  nodeVersion: process.version,
  runtimeVersions: { node: process.versions.node, ...profile },
  seed: initialSeed,
  caseCount: cases.length,
  uniqueInputs: new Set(cases.map(c => c.input)).size,
  scope: 'Special URL serialization, excluding nonempty fragments and deliberate GS1 primary-path dot preservation. This is not full IDNA conformance.',
  cases,
}));
