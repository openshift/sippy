// Synthetic evidence intentionally includes near matches to detect context loss.
export const RELEASE = '5.0'
export const JOB = 'periodic-synthetic-aws'
export const TEST = '[sig-synthetic] routes / paths? a&b=#50% + %2F café'
export const CLUSTER = 'build-synthetic-a'
export const ORG = 'synthetic-org'
export const REPO = 'synthetic-repo'
export const GATE = 'SyntheticGate'
export const RUN = '99001'
export const PR_NUMBER = 98765
export const INTERVAL_FILE = 'synthetic-intervals.json'
export const EMPTY_INTERVAL_FILE = 'missing-intervals.json'
export const PROW = `https://prow.example.invalid/view/gs/synthetic/${RUN}`
export const GCS = `https://storage.example.invalid/synthetic/${RUN}`
export const NOW = '2026-10-26T12:00:00Z'

export function statistics() {
  return {
    current_runs: 20,
    current_successes: 16,
    current_failures: 3,
    current_flakes: 1,
    current_pass_percentage: 80,
    current_working_percentage: 85,
    current_flake_percentage: 5,
    current_failure_percentage: 15,
    previous_runs: 10,
    previous_successes: 9,
    previous_failures: 1,
    previous_flakes: 0,
    previous_pass_percentage: 90,
    previous_working_percentage: 90,
    previous_flake_percentage: 0,
    previous_failure_percentage: 10,
    net_improvement: -10,
    net_working_improvement: -5,
  }
}

export function jobs() {
  return Array.from({ length: 27 }, (_, i) => ({
    ...statistics(),
    id: i + 1,
    name: i === 0 ? JOB : `periodic-synthetic-${String(i).padStart(2, '0')}`,
    org: ORG,
    repo: i === 26 ? 'another-repo' : REPO,
    variants: ['Platform:aws', 'Architecture:amd64'],
    current_pass_percentage: i === 1 ? 0 : 80,
    previous_pass_percentage: i === 1 ? 0 : 90,
    last_pass: '2026-10-24T12:00:00Z',
    open_bugs: 0,
    average_retests_to_merge: 1.2,
  }))
}

export function tests() {
  return [
    TEST,
    `${TEST} near match`,
    'another synthetic test',
    'install should succeed: infrastructure',
  ].map((name, i) => ({
    ...statistics(),
    id: i + 1,
    name,
    suite_name: 'synthetic-suite',
    variants: ['Platform:aws', 'Architecture:amd64'],
    tags: [],
    bugs: [],
    jira_component: 'Synthetic',
    lifecycles: ['blocking'],
  }))
}

export function runs() {
  return Array.from({ length: 27 }, (_, i) => ({
    id: Number(RUN) + i,
    job: i === 26 ? 'periodic-other-cluster' : JOB,
    name: i === 26 ? 'periodic-other-cluster' : JOB,
    cluster: i === 26 ? 'build-synthetic-b' : CLUSTER,
    timestamp: `2026-10-25T12:${String(i).padStart(2, '0')}:00Z`,
    overall_result: i === 0 ? 'F' : 'S',
    test_failures: i === 0 ? 1 : 0,
    test_flakes: 0,
    failed_test_names: i === 0 ? [TEST] : [],
    test_names: [TEST],
    variants: ['Platform:aws', 'Architecture:amd64'],
    labels: [],
    url: i === 0 ? PROW : `https://prow.example.invalid/run/${Number(RUN) + i}`,
    brief_name: JOB,
  }))
}

export function labels() {
  return [
    {
      id: 'synthetic-a',
      label_title: 'Alpha label',
      explanation: 'Alpha evidence',
      bugs: [],
      hide_display_contexts: [],
    },
    {
      id: 'synthetic-b',
      label_title: 'Beta label',
      explanation: 'Beta evidence',
      bugs: ['OCPBUGS-99999'],
      hide_display_contexts: ['metrics'],
      links: { self: '/api/jobs/labels/synthetic-b' },
    },
  ]
}

export function health(empty = false) {
  return {
    indicators: Object.fromEntries(
      [
        'infrastructure',
        'installConfig',
        'bootstrap',
        'installOther',
        'install',
        'upgrade',
        'tests',
      ].map((name) => [
        name,
        {
          ...statistics(),
          ...(empty ? { current_runs: 0, previous_runs: 0 } : {}),
        },
      ])
    ),
  }
}

export function operatorResults(empty = false) {
  return {
    column_names: ['aws'],
    tests: empty ? {} : { [TEST]: { aws: { ...statistics() } } },
  }
}

export function gateDetail() {
  return {
    feature_gate: GATE,
    release: RELEASE,
    first_seen_in: '4.22',
    unique_test_count: 1,
    matching_jobs: [JOB],
    promotion: {
      sufficient: false,
      warnings: [],
      errors: [],
      results_by_variant: [
        {
          variants: { Platform: 'aws', Architecture: 'amd64' },
          optional: false,
          test_results: [
            { test_name: TEST, total_runs: 20, pass_percent: 0.8 },
          ],
        },
      ],
    },
  }
}

export function events() {
  return ['selected-pod', 'other-pod'].map((name, i) => ({
    kind: 'Pod',
    namespace: 'synthetic-ns',
    name,
    type: i ? 'Normal' : 'Warning',
    reason: i ? 'Started' : 'Failed',
    message: i ? 'Other evidence' : 'Synthetic selected evidence',
    count: 1,
    firstTimestamp: `2026-10-25T12:0${i}:00Z`,
    lastTimestamp: `2026-10-25T12:0${i}:00Z`,
  }))
}

export function intervals() {
  return ['selected-pod', 'other-pod'].map((name) => ({
    from: '2026-10-25T12:00:00Z',
    to: '2026-10-25T12:05:00Z',
    source: 'Pod',
    level: 'Info',
    display: true,
    filename: INTERVAL_FILE,
    locator: { keys: { ns: 'synthetic-ns', pod: name } },
    message: { annotations: {}, humanMessage: `Evidence for ${name}` },
  }))
}
