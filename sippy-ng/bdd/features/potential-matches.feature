Feature: Potential match dialogs
  As a Component Readiness maintainer
  I want to link regressions and triages that look alike
  So that one Jira issue covers them without dropping the members a triage already has

  There are two entry points. The triage dialog on the regressed tests dialog suggests triages for one regression.
  The triage detail page suggests regressions for one triage.

  Scenario: Adding a regression to a matching triage keeps that triage's members
    Given a component readiness API
    And a triage exists for the other regression
    And the regression has a matching triage
    When I open the component readiness report
    And I open the regressed tests dialog
    And I open the triage dialog with potential matches
    Then the matching triage "pod network regression" has confidence "6/10"
    And the matching triage links to triage 7
    When I add the regression to the matching triage
    Then the triage update keeps both regressions
    And the matching triage success message is shown

  Scenario: A rejected add to a matching triage does not claim success
    Given a component readiness API
    And a triage exists for the other regression
    And the regression has a matching triage
    And the next mutation is rejected
    When I open the component readiness report
    And I open the regressed tests dialog
    And I open the triage dialog with potential matches
    And I add the regression to the matching triage
    Then the matching triage error is shown without a success message

  Scenario: A failed match lookup is shown as an error
    Given a component readiness API
    And the regression match lookup fails
    When I open the component readiness report
    And I open the regressed tests dialog
    And I open the triage dialog with potential matches
    Then the match lookup error is shown
    And no mutation requests were sent

  Scenario: Linking matching regressions keeps the triage's existing members
    Given a component readiness API
    And a triage with regressions in two views exists
    And the triage has potential matching regressions
    When I open triage 7
    And I open the potential matching regressions
    Then the potential matches were requested for view "5.0-main"
    And the potential matches show "should maintain pod network after upgrade"
    And the potential matches show "should keep service endpoints"
    And the potential matches do not show "should keep node network"
    When I turn off the "Similar Names" match filter
    Then the potential matches show "should keep service endpoints"
    And the potential matches do not show "should maintain pod network after upgrade"
    When I turn on the "Already Triaged" match filter
    Then the potential matches show "should keep node network"
    When I switch the potential matches view to "5.0-other"
    Then the potential matches were requested for view "5.0-other"
    And the potential matches do not show "should keep node network"
    When I select the potential match "should keep service endpoints"
    And I add the selected matches to the triage
    Then the triage link request keeps regression 101 and adds 302
    And the potential matches dialog is closed
    And the included tests show "should keep service endpoints"
    And the included tests show "should maintain pod network"

  Scenario: A rejected link does not claim the regressions were added
    Given a component readiness API
    And a triage with regressions in two views exists
    And the triage has potential matching regressions
    And the next mutation is rejected
    When I open triage 7
    And I open the potential matching regressions
    And I select the potential match "should keep service endpoints"
    And I add the selected matches to the triage
    Then the page shows "Error linking regressions: Failed to link regressions: 400"
    And triage 7 still has only regression 101

  Scenario: A triage whose regressions rolled off cannot look for matches
    Given a component readiness API
    And a triage whose regressions rolled off exists
    When I open triage 7
    Then looking for matching regressions is disabled because they rolled off
    And no potential matches were requested

  Scenario: A new triage with potential matches offers to open them
    Given a component readiness API
    And the triage has potential matching regressions
    When I open the component readiness report
    And I open the regressed tests dialog
    And I create a triage for the selected regression
    Then the new triage offers 3 potential matches
    And the potential matches were checked for triage 8
    When I choose to view the matches in a new tab
    Then the triage success message is shown
    And the new tab shows the potential matching regressions for triage 8
