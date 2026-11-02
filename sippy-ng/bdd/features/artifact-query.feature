Feature: Job artifact comparison and symptoms
  As a Component Readiness maintainer
  I want to search the artifacts of the runs I picked on test details
  So that the evidence, symptoms, and labels I record come from those runs and no others

  Scenario: Compare Sample Failures searches only the failed sample runs
    Given a component readiness API
    When I open the test details deep link
    And I compare the sample failures
    Then the artifact query lists only job run "9001"
    When I search artifacts with the path glob "build-log.txt"
    Then the latest artifact request searches runs "9001" with the path glob "build-log.txt"
    And the latest artifact request has no content match
    And job run "9001" lists the artifact "build-log.txt"

  Scenario: Compare Selected searches the runs I picked
    Given a component readiness API
    And test details include a failed basis run
    When I open the test details deep link
    And I choose runs to compare
    And I toggle the failed runs of job "periodic-ci-base"
    And I toggle the failed runs of job "periodic-ci-sample"
    And I compare the selected runs
    Then the artifact query lists only job run "8001"
    When I search artifacts with the path glob "build-log.txt"
    Then the latest artifact request searches runs "8001" with the path glob "build-log.txt"

  Scenario: A string search sends the match and context settings
    Given a component readiness API
    When I open the test details deep link
    And I compare the sample failures
    And I search artifacts with the path glob "build-log.txt"
    And I match artifact content with the string "level=error"
    Then the latest artifact request includes "textContains=level=error"
    And the latest artifact request includes "maxFileMatches=12"
    And the latest artifact request includes "beforeContext=0"
    And the latest artifact request includes "afterContext=0"
    And the matched artifact line is shown

  Scenario: A regex search sends the regex
    Given a component readiness API
    When I open the test details deep link
    And I compare the sample failures
    And I search artifacts with the path glob "build-log.txt"
    And I match artifact content with the regex "etcd leader .*"
    Then the latest artifact request includes "textRegex=etcd leader .*"
    And the latest artifact request does not include "textContains="
    And the matched artifact line is shown

  Scenario: A search with no artifacts says nothing was found
    Given a component readiness API
    And artifact searches find nothing
    When I open the test details deep link
    And I compare the sample failures
    And I search artifacts with the path glob "build-log.txt"
    And I show only runs with artifacts
    Then the artifact query says no results were found
    And no artifact is listed

  Scenario: A failed search is shown as an error
    Given a component readiness API
    And artifact searches fail
    When I open the test details deep link
    And I compare the sample failures
    And I search artifacts with the path glob "build-log.txt"
    Then the artifact query shows a search error
    And no artifact is listed

  Scenario: Saving a symptom records the label and this search
    Given a component readiness API
    When I open the test details deep link
    And I compare the sample failures
    And I search artifacts with the path glob "build-log.txt"
    And I match artifact content with the string "level=error"
    And I open the save as symptom section
    And I create the label "etcd leader election"
    Then the label create request is for "etcd leader election"
    And the message "Label \"etcd leader election\" created successfully!" is shown
    When I save the symptom "etcd leader changed"
    Then the symptom create request records "etcd leader changed" for this search and label
    When I open the save as symptom section
    Then the save as symptom section is editing "etcd leader changed"

  Scenario: A rejected symptom save does not claim success
    Given a component readiness API
    And the next mutation is rejected
    When I open the test details deep link
    And I compare the sample failures
    And I search artifacts with the path glob "build-log.txt"
    And I match artifact content with the string "level=error"
    And I open the save as symptom section
    And I save the symptom "etcd leader changed"
    Then the message "Failed to create symptom" is shown
    And the message "Symptom created successfully!" is not shown

  Scenario: Re-evaluation targets the selected run
    Given a component readiness API
    And test details include a failed basis run
    When I open the test details deep link
    And I choose runs to compare
    And I toggle the failed runs of job "periodic-ci-base"
    And I compare the selected runs
    Then the artifact query lists job run "8001"
    And the artifact query lists job run "9001"
    When I select job run "8001" in the artifact results
    And I re-evaluate symptoms
    Then the re-evaluation request targets only run "8001"
    And the message "Successfully re-evaluated 1 job run(s)." is shown

  Scenario: Read-only mode offers no symptom or re-evaluation writes
    Given a read-only component readiness API
    When I open the test details deep link
    And I compare the sample failures
    And I search artifacts with the path glob "build-log.txt"
    Then job run "9001" lists the artifact "build-log.txt"
    And symptom and re-evaluation controls are hidden
    And no mutation requests were sent
