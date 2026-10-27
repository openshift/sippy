export const VIEW = '5.0-main'
export const PREVIOUS_RELEASE = '4.21'
export const BASE_RELEASE = '4.22'
export const SAMPLE_RELEASE = '5.0'
export const ALT_SAMPLE_RELEASE = '5.1'
export const COMPONENT = 'Networking'
export const OTHER_COMPONENT = 'Storage'
export const ALT_COMPONENT = 'Console'
export const CAPABILITY = 'sdn'
export const TEST_ID = 'test-sdn-pod-network'
export const TEST_NAME = 'should maintain pod network'
export const TEST_SUITE = 'openshift-tests'
export const REGRESSION_ID = 101
export const SECOND_REGRESSION_ID = 102
export const FAILED_RUN_ID = '9001'
export const PASSED_RUN_ID = '9002'
export const JIRA_URL = 'https://redhat.atlassian.net/browse/TRT-2631'
export const COLUMN_VARIANTS = { Platform: 'aws', Architecture: 'amd64' }

export const GENERATED_AT = '2026-10-26T12:00:00Z'
export const PINNED_DATE = '2026-10-26T00:00:00Z'
const statsStart = '2026-10-19T00:00:00Z'
const statsEnd = '2026-10-26T00:00:00Z'
const lastFailure = '2026-10-25T00:00:00Z'
const regressionOpened = '2026-10-21T00:00:00Z'

export function releaseCatalog() {
  return {
    releases: [BASE_RELEASE, SAMPLE_RELEASE, ALT_SAMPLE_RELEASE],
    ga_dates: { [BASE_RELEASE]: '2026-06-01' },
    dates: {},
    last_updated: GENERATED_AT,
    release_attrs: {
      [BASE_RELEASE]: {
        name: BASE_RELEASE,
        ga: '2026-06-01',
        previous_release: PREVIOUS_RELEASE,
        capabilities: { componentReadiness: true },
      },
      [SAMPLE_RELEASE]: {
        name: SAMPLE_RELEASE,
        previous_release: BASE_RELEASE,
        capabilities: { componentReadiness: true },
      },
      [ALT_SAMPLE_RELEASE]: {
        name: ALT_SAMPLE_RELEASE,
        previous_release: SAMPLE_RELEASE,
        capabilities: { componentReadiness: true },
      },
    },
  }
}

export function views() {
  return [
    {
      name: VIEW,
      base_release: {
        release: BASE_RELEASE,
        start: '2026-05-01T00:00:00Z',
        end: '2026-06-01T00:00:00Z',
      },
      sample_release: {
        release: SAMPLE_RELEASE,
        start: statsStart,
        end: statsEnd,
      },
      test_id_options: {},
      test_filters: {},
      variant_options: {
        column_group_by: { Platform: {}, Architecture: {} },
        db_group_by: { Platform: {}, Architecture: {} },
        include_variants: {
          Platform: ['aws'],
          Architecture: ['amd64'],
        },
      },
      advanced_options: {
        minimum_failure: 3,
        confidence: 95,
        pity_factor: 5,
        pass_rate_required_new_tests: 0,
        pass_rate_required_all_tests: 0,
        ignore_missing: false,
        ignore_disruption: true,
        flake_as_failure: false,
        include_multi_release_analysis: false,
      },
      metrics: { enabled: false },
      regression_tracking: { enabled: true },
      automate_jira: { enabled: false },
      prime_cache: { enabled: false },
    },
  ]
}

export function regressedTest(id = REGRESSION_ID) {
  return {
    component: COMPONENT,
    capability: CAPABILITY,
    test_name: TEST_NAME,
    test_suite: TEST_SUITE,
    test_id: TEST_ID,
    status: -400,
    variants: COLUMN_VARIANTS,
    explanations: ['pass rate dropped from 99% to 80%'],
    last_failure: lastFailure,
    regression: {
      id,
      opened: regressionOpened,
    },
    links: {
      test_details:
        `/api/component_readiness/test_details?testId=${TEST_ID}` +
        `&component=${COMPONENT}&capability=${CAPABILITY}` +
        `&baseRelease=${BASE_RELEASE}&sampleRelease=${SAMPLE_RELEASE}`,
    },
  }
}

function column(status, tests = []) {
  return {
    status,
    variants: COLUMN_VARIANTS,
    regressed_tests: tests,
  }
}

export function componentReport(component, status, tests = []) {
  return {
    generated_at: GENERATED_AT,
    rows: [
      {
        component,
        columns: [column(status, tests)],
      },
    ],
  }
}

export function mainReport() {
  return {
    generated_at: GENERATED_AT,
    warnings: [],
    rows: [
      {
        component: COMPONENT,
        columns: [column(-400, [regressedTest()])],
      },
      {
        component: OTHER_COMPONENT,
        columns: [column(0)],
      },
    ],
  }
}

export function capabilitiesReport() {
  return {
    generated_at: GENERATED_AT,
    rows: [
      {
        component: COMPONENT,
        capability: CAPABILITY,
        columns: [column(-400, [regressedTest()])],
      },
    ],
  }
}

export function testsReport() {
  return {
    generated_at: GENERATED_AT,
    rows: [
      {
        component: COMPONENT,
        capability: CAPABILITY,
        test_id: TEST_ID,
        test_name: TEST_NAME,
        test_suite: TEST_SUITE,
        columns: [column(-400, [regressedTest()])],
      },
    ],
  }
}

export function singleTestReport() {
  return {
    generated_at: GENERATED_AT,
    rows: [
      {
        component: COMPONENT,
        capability: CAPABILITY,
        test_id: TEST_ID,
        test_name: TEST_NAME,
        test_suite: TEST_SUITE,
        columns: [column(-400, [regressedTest()])],
      },
    ],
  }
}

function releaseStats(release, failures) {
  return {
    release,
    Start: statsStart,
    End: statsEnd,
    success_count: 80,
    failure_count: failures,
    flake_count: 1,
    success_rate: failures === 0 ? 0.99 : 0.8,
  }
}

export function testDetailsReport(options = {}) {
  const analyses = [
    {
      status: -400,
      comparison: 'fisher_exact',
      explanations: ['pass rate dropped from 99% to 80%'],
      sample_stats: releaseStats(SAMPLE_RELEASE, 4),
      base_stats: releaseStats(options.baseRelease || PREVIOUS_RELEASE, 0),
      fisher_exact: 0.99,
      last_failure: lastFailure,
      regression: {
        id: REGRESSION_ID,
        opened: regressionOpened,
        triages: [],
      },
      job_stats: [
        {
          sample_job_name: 'periodic-ci-sample',
          base_job_name: 'periodic-ci-base',
          significant: true,
          sample_stats: releaseStats(SAMPLE_RELEASE, 4),
          base_stats: releaseStats(BASE_RELEASE, 0),
          sample_job_run_stats: [
            {
              job_url: `https://prow.example/view/${FAILED_RUN_ID}`,
              job_run_id: FAILED_RUN_ID,
              start_time: lastFailure,
              test_stats: {
                success_count: 0,
                failure_count: 1,
                flake_count: 0,
                success_rate: 0,
              },
              test_failures: 1,
            },
            {
              job_url: `https://prow.example/view/${PASSED_RUN_ID}`,
              job_run_id: PASSED_RUN_ID,
              start_time: '2026-10-24T00:00:00Z',
              test_stats: {
                success_count: 1,
                failure_count: 0,
                flake_count: 0,
                success_rate: 1,
              },
              test_failures: 0,
            },
          ],
          base_job_run_stats: [],
        },
      ],
    },
  ]
  if (options.historical) {
    analyses.push({
      status: 0,
      comparison: 'fisher_exact',
      explanations: ['historical basis did not regress'],
      sample_stats: releaseStats(SAMPLE_RELEASE, 4),
      base_stats: releaseStats(BASE_RELEASE, 0),
      job_stats: [],
    })
  }
  const report = {
    component: COMPONENT,
    capability: CAPABILITY,
    test_id: TEST_ID,
    test_name: TEST_NAME,
    jira_component: 'Networking',
    jira_component_id: 1,
    lifecycle: options.lifecycle || 'informing',
    generated_at: GENERATED_AT,
    analyses,
  }
  if (options.stale) {
    report.links = {
      latest:
        '/api/component_readiness/test_details?testId=' +
        TEST_ID +
        '&component=' +
        COMPONENT,
    }
  }
  return report
}

export function cancelledReport() {
  return {
    generated_at: GENERATED_AT,
    rows: [
      {
        component: 'Cancelled',
        columns: [column(0)],
      },
    ],
  }
}

export function triageRecord(overrides = {}) {
  const id = overrides.id || 7
  return {
    id,
    url: overrides.url || JIRA_URL,
    type: overrides.type || 'product',
    description: overrides.description || 'pod network regression',
    created_at: GENERATED_AT,
    updated_at: GENERATED_AT,
    resolved: overrides.resolved || { Time: null, Valid: false },
    regressions: (
      overrides.regressions || [
        {
          id: REGRESSION_ID,
          test_name: TEST_NAME,
          opened: regressionOpened,
          variants: ['Platform:aws'],
        },
      ]
    ).map((regression) => ({
      last_failure: { Time: null, Valid: false },
      release: SAMPLE_RELEASE,
      ...regression,
    })),
    links: {
      self: `/api/component_readiness/triages/${id}`,
    },
  }
}
