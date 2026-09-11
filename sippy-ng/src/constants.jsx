export const FEATURE_GATE_TEST_THRESHOLDS = {
  success: 5,
  warning: 2,
  error: 1,
}

export const MERGE_FAILURE_THERSHOLDS = {
  success: 1.5,
  warning: 2,
  error: 3,
}

export const INFRASTRUCTURE_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0,
}

export const INSTALL_CONFIG_THRESHOLDS = {
  success: 99,
  warning: 98,
  error: 0,
}

export const BOOTSTRAP_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0,
}

export const INSTALL_OTHER_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0,
}

export const INSTALL_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0,
}

export const UPGRADE_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0,
}

export const VARIANT_THRESHOLDS = {
  success: 80,
  warning: 60,
  error: 0,
}

export const JOB_THRESHOLDS = {
  success: 80,
  warning: 60,
  error: 50,
}

export const BUILD_CLUSTER_THRESHOLDS = {
  success: 80,
  warning: 60,
  error: 50,
}

export const TEST_THRESHOLDS = {
  success: 80,
  warning: 60,
  error: 0,
}

export const COMPONENT_READINESS_THRESHOLDS = {
  success: 5,
  warning: 15,
  error: 999999,
}

export const BLOCKER_SCORE_THRESHOLDS = {
  success: 10,
  warning: 50,
  error: 75,
}

// Saved searches
export const BOOKMARKS = {
  NEW_JOBS: {
    field: 'previous_runs',
    operator: '=',
    value: '0',
  },
  RUN_FEW: {
    field: 'current_runs',
    operator: '<',
    value: '7',
  },
  RUN_1: {
    field: 'current_runs',
    operator: '>=',
    value: '1',
  },
  RUN_2: {
    field: 'current_runs',
    operator: '>=',
    value: '2',
  },
  RUN_7: {
    field: 'current_runs',
    operator: '>=',
    value: '7',
  },
  RUN_10: {
    field: 'current_runs',
    operator: '>=',
    value: '10',
  },
  NO_NEVER_STABLE: {
    field: 'variants',
    not: true,
    operator: 'has entry',
    value: 'never-stable',
  },
  NO_AGGREGATED: {
    field: 'variants',
    not: true,
    operator: 'has entry',
    value: 'aggregated',
  },
  NO_STEP_GRAPH: {
    field: 'name',
    not: true,
    operator: 'contains',
    value: 'step graph.',
  },
  HIGH_DELTA_FROM_PASSING_AVERAGE: {
    field: 'delta_from_passing_average',
    operator: '<=',
    value: '20',
  },
  HIGH_STANDARD_DEVIATION: {
    field: 'passing_standard_deviation',
    operator: '>',
    value: '1',
  },
  NO_100_FLAKE: {
    field: 'current_flake_percentage',
    not: true,
    operator: '=',
    value: '100',
  },
  NO_OPENSHIFT_TESTS_SHOULD_WORK: {
    field: 'name',
    not: true,
    operator: 'contains',
    value: 'openshift-tests should work',
  },
  WITHOUT_OVERALL_JOB_RESULT: {
    field: 'name',
    not: true,
    operator: 'contains',
    value: '.Overall',
  },
  UPGRADE: {
    field: 'name',
    operator: 'contains',
    value: 'upgrade',
  },
  INSTALL: {
    field: 'tags',
    operator: 'contains',
    value: 'install',
  },
  LINKED_BUG: { field: 'bugs', operator: '>', value: '0' },
  NO_LINKED_BUG: { field: 'bugs', operator: '=', value: '0' },
  ASSOCIATED_BUG: {
    field: 'associated_bugs',
    operator: '>',
    value: '0',
  },
  NO_ASSOCIATED_BUG: {
    field: 'associated_bugs',
    operator: '=',
    value: '0',
  },
  TRT: { field: 'tags', operator: 'contains', value: 'trt' },
}

// Default filters applied when clicking "Tests" link in the sidebar
export const DEFAULT_TEST_FILTERS = [
  BOOKMARKS.RUN_7,
  BOOKMARKS.NO_NEVER_STABLE,
  BOOKMARKS.NO_AGGREGATED,
  BOOKMARKS.WITHOUT_OVERALL_JOB_RESULT,
  BOOKMARKS.NO_STEP_GRAPH,
  BOOKMARKS.NO_OPENSHIFT_TESTS_SHOULD_WORK,
  BOOKMARKS.NO_100_FLAKE,
]
