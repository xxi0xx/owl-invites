package invitation

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

var exportBaseColumns = []string{
	"invitation_id",
	"household_label",
	"contact_email",
	"contact_phone",
	"preferred_delivery",
	"invitation_source",
	"additional_guest_allowance",
	"guest_id",
	"guest_display_name",
	"guest_origin",
	"guest_attendance",
	"response_state",
	"first_submitted_at",
	"last_submitted_at",
}

// ExportEventCSV creates an organizer reporting export from the current
// invitation domain. It is intentionally not an import reconciliation format.
func (s *Service) ExportEventCSV(ctx context.Context, eventID string) ([]byte, error) {
	households, err := s.store.ListByEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}
	questions, err := s.store.listQuestions(ctx, eventID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(questions, func(i, j int) bool {
		if questions[i].SortOrder == questions[j].SortOrder {
			return questions[i].ID < questions[j].ID
		}
		return questions[i].SortOrder < questions[j].SortOrder
	})

	header := append([]string{}, exportBaseColumns...)
	for _, question := range questions {
		header = append(header, exportQuestionColumn(question))
	}

	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write(defangCSVRecord(header)); err != nil {
		return nil, fmt.Errorf("write invitation export header: %w", err)
	}
	for _, household := range households {
		invitationAnswers := make(map[string]string, len(household.InvitationAnswers))
		for _, answer := range household.InvitationAnswers {
			invitationAnswers[answer.QuestionID] = answer.Answer
		}
		guestAnswers := make(map[string]map[string]string)
		for _, answer := range household.GuestAnswers {
			if guestAnswers[answer.GuestID] == nil {
				guestAnswers[answer.GuestID] = make(map[string]string)
			}
			guestAnswers[answer.GuestID][answer.QuestionID] = answer.Answer
		}
		for _, guest := range household.Guests {
			record := exportBaseRecord(household, guest)
			for _, question := range questions {
				var value string
				if question.Scope == QuestionScopeInvitation {
					value = invitationAnswers[question.ID]
				} else {
					value = guestAnswers[guest.ID][question.ID]
				}
				record = append(record, formatExportAnswer(question, value))
			}
			if err := writer.Write(defangCSVRecord(record)); err != nil {
				return nil, fmt.Errorf("write invitation export row: %w", err)
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush invitation export: %w", err)
	}
	return output.Bytes(), nil
}

func exportBaseRecord(household *Household, guest *Guest) []string {
	invitation := household.Invitation
	response := household.Response
	state, firstSubmitted, lastSubmitted := "no_submitted_response", "", ""
	if response.SubmittedAt != nil {
		state = "submitted"
		firstSubmitted = response.SubmittedAt.UTC().Format(time.RFC3339Nano)
		lastSubmitted = response.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return []string{
		invitation.ID,
		invitation.Label,
		stringValue(invitation.ContactEmail),
		stringValue(invitation.ContactPhone),
		invitation.PreferredDeliveryMethod,
		invitation.Source,
		fmt.Sprintf("%d", invitation.AdditionalGuestAllowance),
		guest.ID,
		guest.Name,
		guest.Origin,
		guest.Attendance,
		state,
		firstSubmitted,
		lastSubmitted,
	}
}

func exportQuestionColumn(question *Question) string {
	prefix := "invitation_answer"
	if question.Scope == QuestionScopeGuest {
		prefix = "guest_answer"
	}
	return fmt.Sprintf("%s:%s [%s]", prefix, question.Label, question.ID)
}

func formatExportAnswer(question *Question, value string) string {
	if question.Type != "checkbox" || value == "" {
		return value
	}
	var selected []string
	if err := json.Unmarshal([]byte(value), &selected); err != nil {
		return value
	}
	return strings.Join(selected, " | ")
}

func defangCSVRecord(record []string) []string {
	result := make([]string, len(record))
	for i, value := range record {
		result[i] = defangCSVCell(value)
	}
	return result
}

// defangCSVCell prevents spreadsheet formula execution while preserving the
// human-visible value. CSV quoting alone does not neutralize formulas.
func defangCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
