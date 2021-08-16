
export const INFRASTRUCTURE_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0
}

export const INSTALL_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0
}

export const UPGRADE_THRESHOLDS = {
  success: 90,
  warning: 85,
  error: 0
}

export const VARIANT_THRESHOLDS = {
  success: 80,
  warning: 60,
  error: 0
}

export const JOB_THRESHOLDS = {
  success: 80,
  warning: 60,
  error: 0
}

export const TEST_THRESHOLDS = {
  success: 80,
  warning: 60,
  error: 0
}

// Saved searches
export const BOOKMARKS = {
  RUN_1: { id: 1, columnField: 'current_runs', operatorValue: '>=', value: '1' },
  RUN_10: { id: 1, columnField: 'current_runs', operatorValue: '>=', value: '10' },
  UPGRADE: { id: 2, columnField: 'tags', operatorValue: 'contains', value: 'upgrade' },
  INSTALL: { id: 3, columnField: 'tags', operatorValue: 'contains', value: 'install' },
  LINKED_BUG: { id: 4, columnField: 'bugs', operatorValue: '>', value: '0' },
  NO_LINKED_BUG: { id: 5, columnField: 'bugs', operatorValue: '=', value: '0' },
  ASSOCIATED_BUG: { id: 6, columnField: 'associated_bugs', operatorValue: '>', value: '0' },
  NO_ASSOCIATED_BUG: { id: 7, columnField: 'associated_bugs', operatorValue: '=', value: '0' },
  TRT: { id: 8, columnField: 'tags', operatorValue: 'contains', value: 'trt' }
}
