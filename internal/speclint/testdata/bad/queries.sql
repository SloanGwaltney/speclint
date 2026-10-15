-- name: ListReminders :many
-- @then [features/call-reminders.feature] I receive a reminder soon
SELECT * FROM reminders;
