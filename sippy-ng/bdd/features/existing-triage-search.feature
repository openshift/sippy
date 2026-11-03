Feature: Search existing triages
  Existing triages are filtered as I type, even when separate records have the same label.

  Scenario Outline: Filter duplicate triage labels from <entry>
    Given a component readiness API
    And existing triages have duplicate labels
    When I open existing triages from "<entry>"
    Then all searchable triages are listed
    When I search existing triages for "storage"
    Then only the storage triage is listed
    When I search existing triages for "NETWORK"
    Then only the duplicate network triages are listed
    When I search existing triages for "no matching triage"
    Then no existing triages match
    When I search existing triages for ""
    Then all searchable triages are listed
    When I search existing triages for "TRT-9999"
    Then only the storage triage is listed
    When I select the storage triage and add the regression
    Then the storage triage received the regression

    Examples:
      | entry           |
      | regressed tests |
      | test details    |
