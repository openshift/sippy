Feature: Triage from the regressed tests dialog
  As a Component Readiness maintainer
  I want to triage the regressions I selected in the regressed tests dialog
  So that an accepted triage shows up in the dialog, and a cancelled or rejected one does not

  This dialog opens from the main report, except for one scenario that starts on test details and returns here. It is not the triage detail page.

  Scenario: Creating a triage refreshes membership
    Given a component readiness API
    When I open the component readiness report
    And I open the regressed tests dialog
    Then the untriaged list shows the fixture test
    When I create a triage for the selected regression
    Then the triage success message is shown
    And the triage create request includes the regression
    When I open the "untriaged" regressed tab
    Then the untriaged list does not show the fixture test
    And the regressed tests badge shows 1
    When I open the "triaged" regressed tab
    Then the triaged list shows the new triage

  Scenario: An incomplete triage is not submitted
    Given a component readiness API
    When I open the component readiness report
    And I open the regressed tests dialog
    And I submit an empty triage
    Then the triage form shows validation errors
    And the triage was not created

  Scenario: Cancelling a duplicate Jira URL does not create a triage
    Given a component readiness API
    And an unresolved triage exists
    When I open the component readiness report
    And I open the regressed tests dialog
    And I cancel a duplicate triage URL
    Then the triage was not created

  Scenario: Confirming a duplicate Jira URL creates the triage
    Given a component readiness API
    And an unresolved triage exists
    When I open the component readiness report
    And I open the regressed tests dialog
    And I confirm a duplicate triage URL
    Then the triage create request includes the regression

  Scenario: Closing the dialog does not write
    Given a component readiness API
    When I open the component readiness report
    And I open the regressed tests dialog
    And I open the new triage form
    And I close the triage dialog without saving
    Then no mutation requests were sent

  Scenario: Switching tabs clears the previous selection
    Given a component readiness API
    When I open the component readiness report
    And I open the regressed tests dialog
    And I select a regressed test row
    And I open the "untriaged" regressed tab
    Then the selected regressed row was cleared
    When I open the "unresolved" regressed tab
    And I check the regression for triage
    And I open the "all" regressed tab
    And I open the "unresolved" regressed tab
    Then the triage selection was cleared

  Scenario: Adding to an existing triage keeps its other regressions
    Given a component readiness API
    And the report lists the fixture regression and another
    And a triage exists for the other regression
    When I open the component readiness report
    And I open the regressed tests dialog
    And I add the selected regression to the existing triage
    Then the triage update keeps both regressions
    And the existing triage success message is shown

  Scenario: Triaging from test details refreshes the regressed tests dialog
    Given a component readiness API
    When I open the test details deep link
    And I triage the test from its details
    And I open the component readiness report
    And I open the regressed tests dialog
    And I open the "untriaged" regressed tab
    Then the untriaged list does not show the fixture test
    When I open the "triaged" regressed tab
    Then the triaged list shows the new triage

  Scenario: Each regressed tab shows a distinct fixture set
    Given a component readiness API
    And the report lists distinct regression tabs
    And a triage exists for the other regression
    When I open the component readiness report
    Then the regressed tests badge shows 2
    When I open the regressed tests dialog
    Then the "unresolved" regressed tab shows "should maintain pod network"
    And the "unresolved" regressed tab shows "should keep routes"
    And the "unresolved" regressed tab does not show "should recover after a flake"
    And the "untriaged" regressed tab shows "should maintain pod network"
    And the "untriaged" regressed tab shows "should recover after a flake"
    And the "untriaged" regressed tab does not show "should keep routes"
    And the "all" regressed tab shows "should maintain pod network"
    And the "all" regressed tab shows "should keep routes"
    And the "all" regressed tab shows "should recover after a flake"
    When I open the "triaged" regressed tab
    Then the triaged list shows the new triage

  Scenario: Regressed dialog page and filters stay in the URL
    Given a component readiness API
    And the report lists more regressions than one page
    When I open the component readiness report directly
    And I open the regressed tests dialog
    And I open the next regressed page
    Then the regressed page is 1
    When I search the regressed tests for "paged regression 01"
    And I open the "all" regressed tab
    Then the regressed filters stay in the URL
    And the "all" regressed tab shows "paged regression 01"

  Scenario: A rejected triage create does not claim success
    Given a component readiness API
    And the next mutation is rejected
    When I open the component readiness report
    And I open the regressed tests dialog
    And I create a triage for the selected regression
    Then the triage error is shown without a success message
