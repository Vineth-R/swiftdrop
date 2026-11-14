#!/usr/bin/env node
// Opt-in live Gemini smoke test. It calls the public AI gateway with a harmless
// prompt and verifies the service answers. Normal CI never runs it because it
// needs a configured provider and may incur provider cost.
//
//   AI_LIVE_SMOKE=1 BASE_URL=http://localhost:8080 node scripts/ai-live-smoke.mjs
if (process.env.AI_LIVE_SMOKE !== '1') {
  console.error('set AI_LIVE_SMOKE=1 to run the live provider smoke test');
  process.exit(2);
}
const base = (process.env.BASE_URL || 'http://localhost:8080').replace(
  /\/$/,
  '',
);
const response = await fetch(`${base}/api/v1/ai/chat`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ message: 'Reply with the single word SwiftDrop.' }),
});
const text = await response.text();
if (!response.ok) {
  console.error(
    `AI live smoke failed: HTTP ${response.status}: ${text.slice(0, 300)}`,
  );
  process.exit(1);
}
console.log(`AI live smoke passed: HTTP ${response.status}`);
console.log(text.slice(0, 500));
