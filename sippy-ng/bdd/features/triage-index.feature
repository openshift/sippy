Feature: Triage index and details
  As a Component Readiness maintainer
  I want to find a triage, open it, and read its evidence
  So that I act on the right record with its Jira issue, history, and failure labels in view

  The index is the All Triage Records page. The details are the triage detail page, read without changing anything.

  Scenario: The index lists triages and searches by description
    Given a component readiness API
    And an unresolved triage exists
    And a resolved triage exists for another regression
    When I open the triage index
    Then the triage index shows "pod network regression"
    And the triage index shows "route flake fixed upstream"
    And the Jira link for "pod network regression" opens "https://redhat.atlassian.net/browse/TRT-2631"
    When I search the triage index for "route flake"
    Then the triage index shows "route flake fixed upstream"
    And the triage index does not show "pod network regression"

  Scenario: The resolved bookmark keeps only resolved triages
    Given a component readiness API
    And an unresolved triage exists
    And a resolved triage exists for another regression
    When I open the triage index
    And I apply the "Resolved" triage bookmark
    Then the triage index shows "route flake fixed upstream"
    And the triage index does not show "pod network regression"

  Scenario: Selecting a triage on the index shows its included tests
    Given a component readiness API
    And an unresolved triage exists
    And a resolved triage exists for another regression
    When I open the triage index
    And I select the triage "route flake fixed upstream"
    Then the included tests show "should keep routes"
    And the included tests do not show "should maintain pod network"
    And the details link for "route flake fixed upstream" opens triage 8

  Scenario: An empty index says there are no triages
    Given a component readiness API
    When I open the triage index
    Then the triage index has no rows

  Scenario: A failed index load is shown as a failure
    Given a component readiness API
    And the triage list fails
    When I open the triage index without waiting for rows
    Then the page shows "Error: API server returned 500"

  Scenario: An unresolved triage shows its state, Jira link, and test link
    Given a component readiness API
    And an unresolved triage exists
    When I open triage 7
    Then the triage is shown as not resolved
    And the triage Jira link opens "https://redhat.atlassian.net/browse/TRT-2631"
    And the included tests show "should maintain pod network"
    And the included test "should maintain pod network" links to its test analysis

  Scenario: A resolved triage shows the resolved indicator
    Given a component readiness API
    And a resolved triage exists for another regression
    When I open triage 8
    Then the triage is shown as resolved on "2026-10-23"

  Scenario: A resolved triage whose regression still fails shows a failed fix
    Given a component readiness API
    And a resolved triage with a failed fix exists
    When I open triage 9
    Then the triage is shown as a failed fix
    And the included test status for view "5.0-main" links to its test details

  Scenario: Audit history lists the recorded changes
    Given a component readiness API
    And an unresolved triage exists
    When I open triage 7
    And I open the triage audit logs
    Then the audit logs were requested for triage 7
    And the audit log shows the description change by "developer@example.com"
    And the audit log shows the creation by "developer@example.com"

  Scenario: An empty audit history says so
    Given a component readiness API
    And an unresolved triage exists
    And the audit history is empty
    When I open triage 7
    And I open the triage audit logs
    Then the audit log says there is no history

  Scenario: A failed audit history load is shown as an error
    Given a component readiness API
    And an unresolved triage exists
    And the audit history fails
    When I open triage 7
    And I open the triage audit logs
    Then the audit log shows a load error

  Scenario: Failure labels filter the included tests
    Given a component readiness API
    And a triage with a labelled failed run exists
    When I open triage 7
    Then the failure labels list "etcd leader timeout" for 1 regression
    And the "etcd leader timeout" label links to its Jira bug "OCPBUGS-70001"
    And the "etcd leader timeout" job runs link filters runs by that label
    When I filter regressions to the "etcd leader timeout" label
    Then the included tests are filtered by "etcd leader timeout"
    And the included tests show "should maintain pod network"
    And the included tests do not show "should keep routes"
    When I clear the label filter
    Then the included tests show "should maintain pod network"
    And the included tests show "should keep routes"

  Scenario: A triage without labelled runs does not look up labels
    Given a component readiness API
    And an unresolved triage exists
    When I open triage 7
    Then the failure labels say none are applied
    And no job labels were requested

  Scenario: A missing triage is shown as unavailable
    Given a component readiness API
    When I open the missing triage
    Then the page shows "Error: API server returned 404"
    And the triage details are not shown
