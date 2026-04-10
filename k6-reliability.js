import http from 'k6/http';
import { check, sleep } from 'k6';

// 用法：
// k6 run -e BASE_URL=http://localhost:8080 -e TOKEN=xxx k6-reliability.js

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.TOKEN || '';

if (!TOKEN) {
  throw new Error('请通过 -e TOKEN=你的JWT 传入 token');
}

export const options = {
  scenarios: {
    publish_burst: {
      executor: 'ramping-arrival-rate',
      startRate: 5,
      timeUnit: '1s',
      preAllocatedVUs: 50,
      maxVUs: 300,
      stages: [
        { target: 20, duration: '30s' },
        { target: 60, duration: '60s' },
        { target: 120, duration: '60s' },
        { target: 20, duration: '30s' },
      ],
      exec: 'publishFeed',
    },
    comment_burst: {
      executor: 'constant-arrival-rate',
      rate: 50,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 100,
      maxVUs: 300,
      exec: 'commentFeed',
      startTime: '10s',
    },
    message_burst: {
      executor: 'constant-arrival-rate',
      rate: 40,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 100,
      maxVUs: 300,
      exec: 'sendMessage',
      startTime: '20s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.2'],
    http_req_duration: ['p(95)<1200'],
    'checks{type:rl_or_success}': ['rate>0.95'],
  },
};

const commonHeaders = {
  Authorization: `Bearer ${TOKEN}`,
  'Content-Type': 'application/json',
};

// 你可以通过环境变量覆盖：
// -e COMMENT_FEED_ID=1
// -e TARGET_USER_ID=2
const COMMENT_FEED_ID = Number(__ENV.COMMENT_FEED_ID || 1);
const TARGET_USER_ID = Number(__ENV.TARGET_USER_ID || 2);

export function publishFeed() {
  const payload = JSON.stringify({
    content: `k6 publish ${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    images: '',
    videos: '',
  });

  const res = http.post(`${BASE_URL}/api/feeds`, payload, { headers: commonHeaders, tags: { type: 'rl_or_success' } });

  check(res, {
    'publish: status is 200 or 429': (r) => r.status === 200 || r.status === 429,
  }, { type: 'rl_or_success' });

  sleep(Math.random() * 0.2);
}

export function commentFeed() {
  const payload = JSON.stringify({
    content: `k6 comment ${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
  });

  const res = http.post(`${BASE_URL}/api/feeds/${COMMENT_FEED_ID}/comments`, payload, { headers: commonHeaders, tags: { type: 'rl_or_success' } });

  check(res, {
    'comment: status is 200 or 429': (r) => r.status === 200 || r.status === 429,
  }, { type: 'rl_or_success' });

  sleep(Math.random() * 0.2);
}

export function sendMessage() {
  const payload = JSON.stringify({
    to_user_id: TARGET_USER_ID,
    content: `k6 msg ${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
  });

  const res = http.post(`${BASE_URL}/api/messages`, payload, { headers: commonHeaders, tags: { type: 'rl_or_success' } });

  check(res, {
    'message: status is 200 or 429': (r) => r.status === 200 || r.status === 429,
  }, { type: 'rl_or_success' });

  sleep(Math.random() * 0.2);
}

export function handleSummary(data) {
  const m = data.metrics;
  const summary = {
    checks_pass_rate: m.checks ? m.checks.passes / (m.checks.passes + m.checks.fails) : null,
    req_failed_rate: m.http_req_failed ? m.http_req_failed.values.rate : null,
    req_p95_ms: m.http_req_duration ? m.http_req_duration.values['p(95)'] : null,
    req_count: m.http_reqs ? m.http_reqs.values.count : null,
  };

  return {
    stdout: `\n=== k6 summary ===\n${JSON.stringify(summary, null, 2)}\n`,
    'k6-summary.json': JSON.stringify(data, null, 2),
  };
}
