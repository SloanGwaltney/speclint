package main

// @scenario [features/call-reminders.feature#cancelled-no-reminder] No reminder for a cancelled call
// @scenario [#nope] [#sms-reminder] [#nope]
// @scenario [#cancelled-no-reminder]
// @given [features/call-reminders.feature] I had a call scheduled for 3 PM
// @then [features/call-reminders.feature] I cancelled the call
// @then [features/call-reminders.feature] I receive an SMS reminder
// @then [features/call-reminders.feature#sms-reminder] I receive an SMS reminder
func app() {}
