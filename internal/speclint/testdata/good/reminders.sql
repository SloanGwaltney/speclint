-- name: ListReminders :many
-- @then [features/call-reminders.feature#default-lead-time] I receive a reminder
SELECT * FROM reminders;
