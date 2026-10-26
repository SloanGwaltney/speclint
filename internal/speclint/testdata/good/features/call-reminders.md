# Call reminders

Reminds people before a scheduled call so they don't miss it. Reminders are
local to the device; there's no server push.

## Must

- @id:default-lead-time @status:implemented
  When a call's reminder lead time arrives, the person is reminded of that call.

- @id:shown-in-list @status:implemented Upcoming reminders are listed so they
  can be seen ahead of time.

- @id:snooze @status:not-implemented
  A reminder can be snoozed and comes back later.

- @id:reminder-default-sound @status:deprecated
  A reminder plays the default sound.

- @id:reminder-chosen-sound @status:not-implemented
  A reminder plays the sound chosen in settings.
  Replaces: @id:reminder-default-sound

- @id:sms-reminder @status:withdrawn
  Reminders are sent by SMS.

## Must not

- @id:cancelled-no-reminder @status:implemented
  A cancelled call never produces a reminder.

  Cancellation can arrive after the reminder is queued, so this is checked
  at send time, not schedule time.

  ```text
  - @id:not-an-intent @status:implemented fenced examples aren't read
  ```

## Out of scope

Free-form sections aren't read.

- Calendar sync is handled by the calendar feature.
