/** Measure the M5-owned statement slice in a standard Go coverage profile. */
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const profile = process.argv[2];
assert.ok(
  profile,
  'Usage: node scripts/check-m5-coverage.mjs <go-coverage-profile>',
);
const blocks = new Map();
const m5File =
  /\/apps\/services\/parcel\/(?:main\.go|internal\/(?:parcels\/[^/]+\.go|repository\/[^/]+\.go|http\/parcels\.go|config\/config\.go)):/;
for (const line of readFileSync(profile, 'utf8')
  .trim()
  .split(/\r?\n/)
  .slice(1)) {
  const [block, statementText, hitText] = line.trim().split(/\s+/);
  if (!m5File.test(block)) continue;
  const statements = Number(statementText);
  const hits = Number(hitText);
  assert.ok(
    Number.isFinite(statements) && Number.isFinite(hits),
    'Invalid coverage block',
  );
  const previous = blocks.get(block);
  // -coverpkg may repeat a block across test binaries; count its statements
  // once, and regard it as covered if any test binary exercised the block.
  blocks.set(block, { statements, hits: Math.max(previous?.hits ?? 0, hits) });
}
const total = [...blocks.values()].reduce(
  (sum, block) => sum + block.statements,
  0,
);
const covered = [...blocks.values()].reduce(
  (sum, block) => sum + (block.hits > 0 ? block.statements : 0),
  0,
);
assert.ok(total > 0, 'No M5 statement coverage was recorded');
const percentage = (100 * covered) / total;
console.log(
  `Member 5: ${covered}/${total} statements = ${percentage.toFixed(2)}% (minimum 60%).`,
);
console.log(
  'Slice: Parcel startup, configuration, M5 repository, creation/read/receipt services and parcel HTTP handlers. Excludes reports, tracking, proof and dispatch state machine owned by other members.',
);
assert.ok(percentage >= 60, 'Member 5 statement coverage is below 60%');
