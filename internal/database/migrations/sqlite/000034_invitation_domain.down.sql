DROP TABLE IF EXISTS invitation_recovery_attempts;
DROP TABLE IF EXISTS invitation_recovery_tokens;
DROP TABLE IF EXISTS invitation_sessions;
DROP TABLE IF EXISTS guest_answers;
DROP TABLE IF EXISTS invitation_answers;
DROP TABLE IF EXISTS guest_responses;
DROP TABLE IF EXISTS rsvp_responses;
DROP TABLE IF EXISTS guests;
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS open_enrollments;
ALTER TABLE event_questions DROP COLUMN scope;

