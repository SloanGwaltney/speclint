@status:implemented
Feature: Call reminders

  @status:implemented @id:cancelled-no-reminder
  Scenario: No reminder for a cancelled call
    Given I had a call scheduled for 3:00 PM
    And I cancelled the call
    Then I do not receive a reminder

  @status:implemented
  Scenario: Missing an ID

  @id:no-status
  Scenario: Missing a status

  @status:done @id:Bad_ID
  Scenario: Bad status and ID

  @status:not-implemented @id:no-status
  Scenario: Duplicate ID

  @status:withdrawn @id:sms-reminder
  Scenario: SMS reminder is sent
    Given I have a verified phone number
    Then I receive an SMS reminder
