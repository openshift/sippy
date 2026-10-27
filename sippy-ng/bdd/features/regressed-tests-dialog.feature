Feature: Triage from the regressed tests dialog
  As a Component Readiness maintainer
  I want to triage the regressions I selected in the regressed tests dialog
  So that an accepted triage shows up in the dialog, and a cancelled or rejected one does not

  This dialog opens from the main report. It is not the triage form on the test details report, and it is not the triage detail page.

  Scenario: Creating a triage refreshes membership
    Given a component readiness API
    When I open the component readiness report
    And I open the regressed tests dialog
    Then the untriaged list shows the fixture test
    When I create a triage for the selected regression
    Then the triage success message is shown
    And the triage create request includes the regression
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

  Scenario: A rejected triage create does not claim success
    Given a component readiness API
    And the next mutation is rejected
    When I open the component readiness report
    And I open the regressed tests dialog
    And I create a triage for the selected regression
    Then the triage error is shown without a success message
