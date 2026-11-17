Feature: Historical test details for closed regressions
  As a developer investigating a resolved regression
  I want a link to the test results that led to it after it closes
  So that I do not have to rebuild a historical query by hand

  Closed regressions roll off the current report. The triage table keeps a
  link to a historical report that ends on the regression's last failure date.

  Scenario: A closed regression keeps a historical link after rolling off the report
    Given a component readiness API
    And a triage whose only regression closed exists
    When I open triage 7
    Then the "5.0-main" status column is shown
    And the closed regression links to its historical test details
    And the historical link can be reached from the keyboard

  Scenario: Open and closed regressions in one triage are shown separately
    Given a component readiness API
    And a triage with open and closed regressions exists
    When I open triage 7
    Then the open regression keeps its severity icon
    And the closed regression links to its historical test details
    And the regression closed only in this view says so
    And the regression without a last failure explains that history is unavailable

  Scenario: A historical link identifies the closed regression on test details
    Given a component readiness API
    And the regression API returns the closed regression
    And test details no longer track the regression
    When I open the historical test details link
    Then the closed regression notice shows its closure details
    And the test details assessment is a significant regression
    And the test details request keeps the historical window
    And the regression ID row shows the closed regression
    And the closed regression's triage is listed

  Scenario: Generating the report keeps the historical window and regression
    Given a component readiness API
    And the regression API returns the closed regression
    And test details no longer track the regression
    When I open the historical test details link
    Then the sample date controls show the historical window
    When I generate the report from test details
    Then the page keeps the historical window and regression

  Scenario: A regression that does not match the report is not shown as its context
    Given a component readiness API
    And the regression API returns a regression for another test
    And test details no longer track the regression
    When I open the historical test details link
    Then the notice says the regression does not match this report

  Scenario: A missing regression is reported instead of claiming closure
    Given a component readiness API
    And the regression API cannot find the regression
    And test details no longer track the regression
    When I open the historical test details link
    Then the notice says the regression was not found

  Scenario: Closure details remain when the historical window has no data
    Given test details are empty
    And the regression API returns the closed regression
    When I open the historical test details link
    Then the closed regression notice explains there is no historical data
