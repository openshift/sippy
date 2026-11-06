@workflows
Feature: Label editing and run evidence are scoped safely
  Background:
    Given deterministic Sippy workflow evidence

  Scenario: Select a label, validate edits and save the intended definition
    When I select the second label in the editor
    Then invalid label titles and Jira keys prevent saves
    When I save valid changes to the selected label
    Then only the selected label is updated and success is visible

  Scenario: A rejected label save keeps the draft and shows failure
    When I select the second label in the editor
    Given the next workflow mutation will fail
    When I save valid changes to the selected label
    Then the failed save leaves the original definition and editable draft

  Scenario: Read-only capability disables the label editor
    Given workflow writes are disabled
    When I open workflow page "labels/edit/synthetic-b"
    Then label editing is unavailable and no mutation is sent

  Scenario: Cancel then confirm deletion of the selected label
    When I select the second label in the editor
    And I cancel deleting the selected label
    Then no label definition has been deleted
    When I confirm deleting the selected label
    Then only that label is deleted and success is visible

  Scenario: A rejected delete is distinct from success
    When I select the second label in the editor
    Given the next workflow mutation will fail
    When I confirm deleting the selected label
    Then deletion failure is visible and the selected label remains

  Scenario: Filter selected run intervals and navigate to its events
    When I open and filter the selected run intervals
    Then interval requests and visible evidence retain the requested run
    When I follow the selected run to cluster events and filter evidence
    Then only selected event evidence is shown for the requested run

  Scenario: Selecting a missing interval file clears the previous evidence
    When I open and filter the selected run intervals
    And I select an interval file without evidence
    Then the previous intervals are cleared and the selected file is requested

  Scenario Outline: Missing run evidence never displays another run
    When I open missing run "<kind>"
    Then the missing "<kind>" view contains no selected-run evidence
    Examples:
      | kind      |
      | intervals |
      | events    |

  Scenario: An empty interval object is a missing-data result
    Given the "/api/jobs/runs/intervals" workflow response is "empty"
    When I open workflow page "job_runs/99999/periodic-synthetic-aws/intervals"
    Then the missing "intervals" view contains no selected-run evidence

  Scenario Outline: Failed run evidence identifies the requested run
    Given the "/api/jobs/runs/<kind>" workflow response is "error"
    When I open missing run "<kind>"
    Then I see a workflow error containing "jobRunID=99999"
    Examples:
      | kind      |
      | intervals |
      | events    |
