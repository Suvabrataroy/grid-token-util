/**
 * k6 Load Test: Distributed AI Coding Grid
 *
 * Simulates: 50 org units × 500 workers × 20 tasks/hour
 * SLA: 99th-percentile task assignment latency < 60s
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const NUM_WORKERS = parseInt(__ENV.WORKERS || '50');
const NUM_ORGS = parseInt(__ENV.ORGS || '5');
const TASKS_PER_HOUR = parseInt(__ENV.TASKS_PER_HOUR || '20');

// Custom metrics
const assignmentLatency = new Trend('task_assignment_latency_ms', true);
const taskSuccessRate = new Rate('task_success_rate');
const heartbeatSuccessRate = new Rate('heartbeat_success_rate');
const snapshotLatency = new Trend('dashboard_snapshot_latency_ms', true);

export const options = {
  scenarios: {
    workers: {
      executor: 'constant-vus',
      vus: NUM_WORKERS,
      duration: __ENV.DURATION || '10m',
      exec: 'workerScenario',
    },
    submitters: {
      executor: 'constant-arrival-rate',
      rate: Math.ceil(NUM_ORGS * TASKS_PER_HOUR / 3600),  // tasks per second
      timeUnit: '1s',
      duration: __ENV.DURATION || '10m',
      preAllocatedVUs: NUM_ORGS,
      exec: 'submitterScenario',
    },
    dashboard: {
      executor: 'constant-vus',
      vus: 3,
      duration: __ENV.DURATION || '10m',
      exec: 'dashboardScenario',
    },
  },
  thresholds: {
    // 99th percentile assignment latency < 60s
    'task_assignment_latency_ms{p(99)}': ['p(99)<60000'],
    // Dashboard snapshot < 3s
    'dashboard_snapshot_latency_ms{p(95)}': ['p(95)<3000'],
    // Heartbeat success > 99%
    'heartbeat_success_rate': ['rate>0.99'],
    // Overall HTTP error rate < 1%
    'http_req_failed': ['rate<0.01'],
  },
};

// Shared state (populated in setup)
let orgKeys = [];
let workerRegistrations = [];

export function setup() {
  const orgs = [];
  const keys = [];

  // Create orgs and issue keys
  for (let i = 0; i < NUM_ORGS; i++) {
    const orgName = `load-test-org-${randomString(8)}`;
    const createResp = http.post(`${BASE_URL}/api/v1/orgs`,
      JSON.stringify({ name: orgName, plan_tier: 'pro' }),
      { headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${__ENV.SUPER_KEY || 'superadmin-key'}` } }
    );
    if (createResp.status !== 201) continue;
    const org = createResp.json();

    const keyResp = http.post(`${BASE_URL}/api/v1/api-keys`,
      JSON.stringify({
        org_unit_id: org.id,
        name: `load-key-${i}`,
        scopes: ['tasks:write', 'tasks:read', 'workers:write', 'workers:read', 'outputs:write'],
      }),
      { headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${__ENV.SUPER_KEY || 'superadmin-key'}` } }
    );
    if (keyResp.status !== 201) continue;
    const keyData = keyResp.json();
    keys.push({ orgID: org.id, apiKey: keyData.key });
  }

  return { orgKeys: keys };
}

// Worker scenario: register, then heartbeat loop
export function workerScenario(data) {
  const { orgKeys } = data;
  if (!orgKeys || orgKeys.length === 0) return;

  const org = orgKeys[__VU % orgKeys.length];
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${org.apiKey}`,
  };

  // Register worker
  const agents = ['claude', 'copilot', 'gemini'][__VU % 3];
  const regResp = http.post(`${BASE_URL}/api/v1/workers/register`,
    JSON.stringify({
      hostname_hash: `sha256-worker-${__VU}-${randomString(16)}`,
      agents: [agents],
      capacity_score: randomIntBetween(50, 100),
    }),
    { headers }
  );

  if (regResp.status !== 201) {
    sleep(5);
    return;
  }

  const { worker_id } = regResp.json();
  let taskAssignedAt = null;
  let currentTaskID = null;

  // Heartbeat loop for the duration of the test
  while (true) {
    const state = currentTaskID ? 'busy' : 'idle';
    const hbStart = Date.now();
    const hbResp = http.post(`${BASE_URL}/api/v1/workers/${worker_id}/heartbeat`,
      JSON.stringify({
        state,
        cpu_percent: randomIntBetween(5, 40),
        ram_mb_used: randomIntBetween(256, 2048),
        disk_free_gb: randomIntBetween(10, 100),
        battery_percent: 0,
        jobs_today: randomIntBetween(0, 10),
      }),
      { headers }
    );

    heartbeatSuccessRate.add(hbResp.status === 200);

    if (hbResp.status === 200) {
      const body = hbResp.json();
      if (body.assigned_task && !currentTaskID) {
        currentTaskID = body.assigned_task.task_id;
        taskAssignedAt = Date.now();

        // Record assignment latency if we know when task was submitted
        // (approximate: from heartbeat response time)
        assignmentLatency.add(Date.now() - hbStart);

        // Simulate task execution (2-10 seconds)
        const execTime = randomIntBetween(2000, 10000);
        sleep(execTime / 1000);

        // Mark running
        http.patch(`${BASE_URL}/api/v1/tasks/${currentTaskID}/status`,
          JSON.stringify({ state: 'running' }),
          { headers }
        );

        sleep(execTime / 1000);

        // Mark completed
        const completeResp = http.patch(`${BASE_URL}/api/v1/tasks/${currentTaskID}/status`,
          JSON.stringify({ state: 'completed' }),
          { headers }
        );
        taskSuccessRate.add(completeResp.status === 200);
        currentTaskID = null;
      }
    }

    sleep(30); // 30s heartbeat interval
  }
}

// Task submitter scenario
export function submitterScenario(data) {
  const { orgKeys } = data;
  if (!orgKeys || orgKeys.length === 0) return;

  const org = orgKeys[__VU % orgKeys.length];
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${org.apiKey}`,
  };

  const agents = ['claude', 'copilot', 'gemini'];
  const taskTypes = ['code_generation', 'code_review', 'refactor', 'test_generation', 'bug_fix'];

  http.post(`${BASE_URL}/api/v1/tasks`,
    JSON.stringify({
      title: `Load test task ${randomString(8)}`,
      description: `Implement a ${randomString(12)} function with proper error handling and unit tests`,
      task_type: taskTypes[randomIntBetween(0, taskTypes.length - 1)],
      ai_agent: agents[randomIntBetween(0, agents.length - 1)],
      priority: randomIntBetween(1, 10),
    }),
    { headers }
  );

  // No sleep — arrival rate is controlled by executor
}

// Dashboard monitoring scenario
export function dashboardScenario(data) {
  const { orgKeys } = data;
  if (!orgKeys || orgKeys.length === 0) return;

  const org = orgKeys[0];
  const headers = { 'Authorization': `Bearer ${org.apiKey}` };

  const start = Date.now();
  const snapResp = http.get(`${BASE_URL}/api/v1/dashboard/snapshot`, { headers });
  snapshotLatency.add(Date.now() - start);

  check(snapResp, {
    'snapshot status 200': (r) => r.status === 200,
    'snapshot has workers': (r) => r.json('workers') !== undefined,
    'snapshot has tasks': (r) => r.json('tasks') !== undefined,
  });

  sleep(10); // Check dashboard every 10s
}

export function teardown(data) {
  // Cleanup: nothing needed, test DB will be dropped
}
