Feature: Call reminders
  As a user
  I want to be reminded before a scheduled call
  So that I don't miss it

  @status:implemented @id:cancelled-no-reminder
  Scenario: No reminder for a cancelled call
    Given I had a call scheduled for 3:00 PM
    And I cancelled the call
    When the time reaches 2:45 PM
    Then I do not receive a reminder

  @status:implemented @id:default-lead-time
  Scenario: Reminder fires at the default lead time
    Given I had a call scheduled for 3:00 PM
    When the time reaches 2:45 PM
    Then I receive a reminder

  @status:implemented @id:shown-in-list
  Scenario: Reminder shows in the list
    Given I have a reminder
    Then I receive a reminder

  @status:not-implemented @id:snooze
  Scenario: Snooze a reminder
    Given I have a reminder
    When I snooze it
    Then it fires again in 5 minutes

  @status:withdrawn @id:sms-reminder
  Scenario: SMS reminder is sent
    Given I have a verified phone number
    Then I receive an SMS reminder
