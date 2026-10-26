# Call reminders

## Must

- @id:cancelled-no-reminder @status:implemented
  A cancelled call never produces a reminder.

- @status:implemented
  Missing an ID.

- @id:no-status
  Missing a status.

- @id:Bad_ID @status:done @kind:must
  Bad tags.

- @id:no-status @status:not-implemented
  Duplicate ID.

- @id:sms-reminder @status:withdrawn
  Reminders are sent by SMS.

- @id:no-statement @status:not-implemented

- A plain bullet under Must.

- @id:too-wordy @status:not-implemented
  This statement says far too much for one obligation and should be split up.

- @id:replaces-live @status:not-implemented
  Replaces intents that are still live or missing.
  Replaces: @id:cancelled-no-reminder @id:nope

## Notes

- @id:misplaced @status:implemented
  A tagged item outside Must and Must not.

# Second title
